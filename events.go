package main

import (
	"encoding/json"
	"errors"
	"net"
	"sort"
	"time"
)

const (
	feedLimit     = 1024
	waiterLimit   = 64
	maxWaitMillis = 60000
)

type FeedEvent struct {
	Sequence uint64 `json:"sequence"`
	Run      string `json:"run_id"`
	Stage    string `json:"stage"`
	Status   string `json:"status"`
	Token    string `json:"token"`
}

type waiter struct {
	conn       net.Conn
	q          Request
	repository string
	deadline   time.Time
}

// The feed is not persisted: a new stream after restart makes clients resync
// from a snapshot.
type feed struct {
	stream  string
	seq     uint64
	log     []FeedEvent
	seen    map[string]string
	waiters []*waiter
}

func newFeed(s *Store) *feed {
	f := &feed{stream: opaque(), seen: map[string]string{}}
	for id, r := range s.Runs {
		f.seen[id] = digest(r)
	}
	return f
}

func (f *feed) observe(s *Store) {
	ids := make([]string, 0, len(s.Runs))
	for id := range s.Runs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := s.Runs[id]
		d := digest(r)
		if f.seen[id] == d {
			continue
		}
		f.seen[id] = d
		f.seq++
		f.log = append(f.log, FeedEvent{Sequence: f.seq, Run: id, Stage: r.Stage, Status: r.Status, Token: r.Token})
		if len(f.log) > feedLimit {
			f.log = f.log[len(f.log)-feedLimit:]
		}
	}
}

func (f *feed) since(s *Store, q Request, repository string) (events []FeedEvent, resync bool) {
	if q.Stream != f.stream || q.After > f.seq || len(f.log) > 0 && f.log[0].Sequence > q.After+1 {
		return nil, true
	}
	for _, e := range f.log {
		if r := s.Runs[e.Run]; e.Sequence > q.After && r != nil && r.Submitted.Repository == repository {
			events = append(events, e)
		}
	}
	return events, false
}

func (f *feed) answer(s *Store, q Request, events []FeedEvent, wait string) Response {
	v := handle(s, Request{Version: q.Version, Op: "inspect", Path: q.Path, Host: q.Host})
	v.Stream, v.Sequence, v.Events, v.Wait = f.stream, f.seq, events, wait
	return v
}

func (f *feed) subscribe(s *Store, c net.Conn, q Request) (Response, bool) {
	refuse := func(e error) (Response, bool) {
		v := handle(s, Request{Version: q.Version, Op: "inspect", Path: q.Path})
		v.Outcome, v.Error = "refused", e.Error()
		return v, false
	}
	if e := checkVersion(q); e != nil {
		return refuse(e)
	}
	if q.WaitMS < 0 || q.WaitMS > maxWaitMillis {
		return refuse(errors.New("wait_ms must be between 0 and 60000"))
	}
	id, e := resolve(q.Path)
	if e != nil {
		return refuse(e)
	}
	events, resync := f.since(s, q, id.Repository)
	switch {
	case resync:
		return f.answer(s, q, nil, "resync"), false
	case len(events) > 0:
		return f.answer(s, q, events, "events"), false
	case q.WaitMS == 0:
		return f.answer(s, q, nil, "elapsed"), false
	case len(f.waiters) >= waiterLimit:
		return refuse(errors.New("too many waiting subscribers; retry later"))
	}
	c.SetDeadline(time.Time{})
	f.waiters = append(f.waiters, &waiter{conn: c, q: q, repository: id.Repository, deadline: time.Now().Add(time.Duration(q.WaitMS) * time.Millisecond)})
	return Response{}, true
}

func (f *feed) flush(s *Store, now time.Time) {
	kept := f.waiters[:0]
	for _, w := range f.waiters {
		events, resync := f.since(s, w.q, w.repository)
		var v Response
		switch {
		case resync:
			v = f.answer(s, w.q, nil, "resync")
		case len(events) > 0:
			v = f.answer(s, w.q, events, "events")
		case !now.Before(w.deadline):
			v = f.answer(s, w.q, nil, "elapsed")
		default:
			kept = append(kept, w)
			continue
		}
		stamp(&v, w.q.Version)
		w.conn.SetWriteDeadline(now.Add(2 * time.Second))
		json.NewEncoder(w.conn).Encode(v)
		w.conn.Close()
	}
	f.waiters = kept
}

func (f *feed) next(now time.Time) time.Duration {
	wait := time.Hour
	for _, w := range f.waiters {
		wait = min(wait, max(w.deadline.Sub(now), 0))
	}
	return wait
}

func (f *feed) close() {
	for _, w := range f.waiters {
		w.conn.Close()
	}
	f.waiters = nil
}

type liveUpdate struct {
	state string
	view  *Response
}

func follow(h, path, stream string, after uint64, updates chan liveUpdate, stop <-chan struct{}) {
	for {
		v, err := request(h, Request{Version: protocolMax, Op: "watch", Path: path, Stream: stream, After: after, WaitMS: 25000}, false)
		u := liveUpdate{state: "live"}
		if err == nil && v.Outcome == "ok" {
			stream, after, u.view = v.Stream, v.Sequence, &v
		} else {
			stream, u.state = "", "reconnecting (daemon unavailable)"
		}
		select {
		case <-updates:
		default:
		}
		select {
		case updates <- u:
		case <-stop:
			return
		}
		if u.view == nil {
			select {
			case <-stop:
				return
			case <-time.After(time.Second):
			}
		}
	}
}

type snapshotOrder map[string]bool

// Action responses carry the sequence committed before their own change.
// Only the follower may introduce a stream; action responses may be delayed.
func (o snapshotOrder) accepts(update, current Response, actionResponse bool) bool {
	switch {
	case o[update.Stream]:
		return false
	case update.Stream != current.Stream:
		return !actionResponse
	}
	return update.Sequence > current.Sequence || actionResponse && update.Sequence == current.Sequence
}

func (o snapshotOrder) adopt(next, current Response) {
	if next.Stream != current.Stream && current.Stream != "" {
		o[current.Stream] = true
	}
}

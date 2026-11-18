package engine

import (
	"encoding/json"
	"errors"
	"net"
	"sort"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/forge"
	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

const (
	feedLimit     = 1024
	waiterLimit   = 64
	maxWaitMillis = 60000
)

type FeedEvent struct {
	Checks    []forge.CheckState `json:"checks,omitempty"`
	Kind      string             `json:"kind,omitempty"`
	Execution string             `json:"execution,omitempty"`
	Command   []string           `json:"command,omitempty"`
	ExitCode  *int               `json:"exit_code,omitempty"`
	ElapsedMS *int64             `json:"elapsed_ms,omitempty"`
	Sequence  uint64             `json:"sequence"`
	Run       string             `json:"run_id"`
	Stage     string             `json:"stage"`
	Status    string             `json:"status"`
	Token     string             `json:"token"`
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
	recordStage func(string, StageResult)
	stream      string
	seq         uint64
	log         []FeedEvent
	stageSeen   map[string]string
	seen        map[string]string
	waiters     []*waiter
}

func newFeed(s *Store) *feed {
	f := &feed{stream: opaque(), seen: map[string]string{}, stageSeen: map[string]string{}}
	for id, r := range s.Runs {
		f.seen[id] = Digest(r)
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
		d := Digest(r)
		if f.seen[id] == d {
			continue
		}
		f.seen[id] = d
		if p := r.Pipeline; p != nil {
			for _, stage := range p.Stages {
				key := r.ID + ":" + stage.Name
				digest := Digest(stage)
				if f.stageSeen[key] == digest {
					continue
				}
				f.stageSeen[key] = digest
				if f.recordStage != nil {
					f.recordStage(r.ID, stage)
				}
			}
		}
		kind := "run-changed"
		if r.Pipeline != nil && r.Pipeline.Open() != nil {
			kind = "decision"
		} else if r.Status == "paused" || r.Status == "needs-decision" || r.Status == "recovery-required" {
			kind = "park"
		}
		f.append(FeedEvent{Run: id, Stage: r.Stage, Status: r.Status, Token: r.Token, Kind: kind})
	}
}

func (f *feed) append(event FeedEvent) {
	f.seq++
	event.Sequence = f.seq
	f.log = append(f.log, event)
	if len(f.log) > feedLimit {
		f.log = f.log[len(f.log)-feedLimit:]
	}
}

func (f *feed) since(s *Store, q Request, repository string) (events []FeedEvent, resync bool) {
	if q.Stream != f.stream || q.After > f.seq || len(f.log) > 0 && f.log[0].Sequence > q.After+1 {
		return nil, true
	}
	for _, e := range f.log {
		if r := s.Runs[e.Run]; e.Sequence > q.After && r != nil && r.Submitted.Repository == repository && (q.RunID == "" || q.RunID == r.ID) {
			events = append(events, e)
		}
	}
	return events, false
}

func (f *feed) answer(s *Store, q Request, events []FeedEvent, wait string) Response {
	v := handle(s, Request{Version: q.Version, Op: "inspect", Path: q.Path, RunID: q.RunID, Host: q.Host})
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
	id, e := repository.Resolve(q.Path)
	if e != nil {
		return refuse(e)
	}
	if q.Op == "logs" {
		v := handle(s, q)
		if v.Outcome != "ok" || !q.Follow || v.Logs.Next > v.Logs.Offset || q.WaitMS == 0 || logsComplete(v.Run, q.Step) {
			return v, false
		}
		if len(f.waiters) >= waiterLimit {
			return refuse(errors.New("too many waiting subscribers; retry later"))
		}
		q.RunID = v.RunID
		c.SetDeadline(time.Time{})
		f.waiters = append(f.waiters, &waiter{conn: c, q: q, repository: id.Repository, deadline: time.Now().Add(time.Duration(q.WaitMS) * time.Millisecond)})
		return Response{}, true
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
		if w.q.Op == "pipeline" {
			v := handle(s, Request{Version: w.q.Version, Op: "inspect", Path: w.q.Path, RunID: w.q.RunID, Host: w.q.Host})
			if v.Outcome == "ok" && v.Run != nil && v.Run.Status == "running" && now.Before(w.deadline) {
				kept = append(kept, w)
				continue
			}
			v.Stream, v.Sequence = f.stream, f.seq
			v.Wait = "settled"
			if v.Run != nil && v.Run.Status == "running" {
				v.Wait = "elapsed"
			}
			stamp(&v, w.q.Version)
			w.conn.SetWriteDeadline(now.Add(2 * time.Second))
			json.NewEncoder(w.conn).Encode(v)
			w.conn.Close()
			continue
		}
		if w.q.Op == "logs" {
			v := handle(s, w.q)
			if v.Outcome == "ok" && v.Logs.Next == v.Logs.Offset && !logsComplete(v.Run, w.q.Step) && now.Before(w.deadline) {
				kept = append(kept, w)
				continue
			}
			v.Wait = "elapsed"
			if v.Logs != nil && v.Logs.Next > v.Logs.Offset {
				v.Wait = "output"
			} else if logsComplete(v.Run, w.q.Step) {
				v.Wait = "complete"
			}
			stamp(&v, w.q.Version)
			w.conn.SetWriteDeadline(now.Add(2 * time.Second))
			json.NewEncoder(w.conn).Encode(v)
			w.conn.Close()
			continue
		}
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
		if w.q.Op == "logs" {
			wait = min(wait, 100*time.Millisecond)
		}
	}
	return wait
}

func (f *feed) close() {
	for _, w := range f.waiters {
		w.conn.Close()
	}
	f.waiters = nil
}

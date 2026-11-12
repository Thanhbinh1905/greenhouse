package terminal

import (
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

type liveUpdate struct {
	state string
	view  *engine.Response
}

func follow(h, path, stream string, after uint64, updates chan liveUpdate, stop <-chan struct{}) {
	for {
		v, err := engine.Send(h, engine.Request{Version: engine.MaxVersion, Op: "watch", Path: path, Stream: stream, After: after, WaitMS: 25000}, false)
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
func (o snapshotOrder) accepts(update, current engine.Response, actionResponse bool) bool {
	switch {
	case o[update.Stream]:
		return false
	case update.Stream != current.Stream:
		return !actionResponse
	}
	return update.Sequence > current.Sequence || actionResponse && update.Sequence == current.Sequence
}

func (o snapshotOrder) adopt(next, current engine.Response) {
	if next.Stream != current.Stream && current.Stream != "" {
		o[current.Stream] = true
	}
}

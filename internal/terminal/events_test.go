package terminal

import (
	"testing"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

func TestTerminalIgnoresOlderSnapshots(t *testing.T) {
	current := engine.Response{Stream: "s1", Sequence: 7}
	order := snapshotOrder{}
	for _, c := range []struct {
		update engine.Response
		want   bool
	}{
		{engine.Response{Stream: "s1", Sequence: 7}, false},
		{engine.Response{Stream: "s1", Sequence: 6}, false},
		{engine.Response{Stream: "s1", Sequence: 8}, true},
		{engine.Response{Stream: "s2", Sequence: 1}, true},
	} {
		if got := order.accepts(c.update, current, false); got != c.want {
			t.Fatalf("accepts(%+v) = %v, want %v", c.update, got, c.want)
		}
	}
	if !order.accepts(engine.Response{Stream: "s1", Sequence: 7}, current, true) {
		t.Fatal("an action response at the shown sequence was refused")
	}
	restarted := engine.Response{Stream: "s2", Sequence: 1}
	order.adopt(restarted, current)
	if order.accepts(engine.Response{Stream: "s1", Sequence: 9}, restarted, false) || order.accepts(engine.Response{Stream: "s1", Sequence: 9}, restarted, true) {
		t.Fatal("a snapshot from the replaced stream was accepted")
	}
	if !order.accepts(engine.Response{Stream: "s3", Sequence: 1}, restarted, false) {
		t.Fatal("a snapshot from a later daemon was refused")
	}
}

func TestOnlyFollowerMayIntroduceStream(t *testing.T) {
	order := snapshotOrder{}
	a := engine.Response{Stream: "daemon-A", Sequence: 5}
	b := engine.Response{Stream: "daemon-B", Sequence: 1}
	c := engine.Response{Stream: "daemon-C", Sequence: 1}
	if order.accepts(b, a, true) {
		t.Fatal("action response introduced B before follower resync")
	}
	if !order.accepts(c, a, false) {
		t.Fatal("follower could not introduce C")
	}
	order.adopt(c, a)
	if order.accepts(b, c, true) {
		t.Fatal("delayed unseen B response replaced C")
	}
	if order.accepts(a, c, false) {
		t.Fatal("retired A watch replaced C")
	}
	if !order.accepts(c, c, true) {
		t.Fatal("same-sequence C action response was refused")
	}
	older := c
	older.Sequence = 0
	if order.accepts(older, c, true) {
		t.Fatal("older C action response was accepted")
	}
}

package engine

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestInspectDuringBlockedCommit(t *testing.T) {
	_, repo, home := fixture(t)
	s, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	v := handle(s, Request{Version: 2, Op: "attach", Path: repo})
	if v.Outcome != "ok" {
		t.Fatal(v)
	}
	connections := make(chan net.Conn)
	release := make(chan struct{})
	started := make(chan struct{})
	finished := make(chan error, 1)
	pending := make(chan net.Conn, 64)
	go func() {
		finished <- commitWhileServing(s, connections, pending, func() error { close(started); <-release; return nil })
	}()
	t.Cleanup(func() { close(release); <-finished })
	<-started
	server, client := net.Pipe()
	defer client.Close()
	connections <- server
	client.SetDeadline(time.Now().Add(time.Second))
	if err := json.NewEncoder(client).Encode(Request{Version: 2, Op: "inspect", Path: repo}); err != nil {
		t.Fatal(err)
	}
	var got Response
	if err := json.NewDecoder(client).Decode(&got); err != nil {
		t.Fatalf("inspect blocked on durable write: %v", err)
	}
	if got.Outcome != "ok" || got.RunID != v.RunID {
		t.Fatal(got)
	}
}

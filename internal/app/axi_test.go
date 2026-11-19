package app

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

func TestWatchRetriesReadDuringCommit(t *testing.T) {
	for _, wait := range []string{"1s", "0s"} {
		t.Run(wait, func(t *testing.T) {
			testWatchReadDuringCommit(t, wait)
		})
	}
}

func testWatchReadDuringCommit(t *testing.T, wait string) {
	home := t.TempDir()
	listener, err := net.Listen("unix", filepath.Join(home, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	requests := make(chan engine.Request, 2)
	go func() {
		for i := 0; i < 2; i++ {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Errorf("set test deadline: %v", err)
				return
			}
			var request engine.Request
			if err := json.NewDecoder(conn).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
				return
			}
			requests <- request
			response := engine.Response{Outcome: "ok", Wait: "elapsed", Stream: "stream"}
			if i == 0 {
				response = engine.Response{Outcome: "refused", Error: "storage commit in progress; retry the read request"}
			}
			_ = json.NewEncoder(conn).Encode(response) // Best-effort HTTP test response.
			_ = conn.Close()                           // Best-effort test cleanup.
		}
	}()
	response, err := axiCommand(home, t.TempDir(), []string{"watch", "--stream", "stream", "--after", "7", "--wait", wait})
	if wait == "0s" {
		if err != nil || response.Outcome != "refused" || len(requests) != 1 {
			t.Fatalf("no-wait watch retried a busy read: response=%+v requests=%d error=%v", response, len(requests), err)
		}
		return
	}
	if err != nil || response.Outcome != "ok" {
		t.Fatalf("watch did not retry busy read: %+v %v", response, err)
	}
	first, second := <-requests, <-requests
	if first.Stream != second.Stream || first.After != second.After || second.WaitMS >= first.WaitMS {
		t.Fatalf("read retry lost cursor or wait budget: first=%+v second=%+v", first, second)
	}
}

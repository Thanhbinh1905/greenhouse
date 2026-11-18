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
	home := t.TempDir()
	listener, err := net.Listen("unix", filepath.Join(home, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	requests := make(chan engine.Request, 2)
	go func() {
		for i := 0; i < 2; i++ {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			var request engine.Request
			json.NewDecoder(conn).Decode(&request)
			requests <- request
			response := engine.Response{Outcome: "ok", Wait: "elapsed", Stream: "stream"}
			if i == 0 {
				response = engine.Response{Outcome: "refused", Error: "storage commit in progress; retry the read request"}
			}
			json.NewEncoder(conn).Encode(response)
			conn.Close()
		}
	}()
	response, err := axiCommand(home, t.TempDir(), []string{"watch", "--stream", "stream", "--after", "7", "--wait", "1s"})
	if err != nil || response.Outcome != "ok" {
		t.Fatalf("watch did not retry busy read: %+v %v", response, err)
	}
	first, second := <-requests, <-requests
	if first.Stream != second.Stream || first.After != second.After || second.WaitMS >= first.WaitMS {
		t.Fatalf("read retry lost cursor or wait budget: first=%+v second=%+v", first, second)
	}
}

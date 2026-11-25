package engine

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/daemonutil"
)

func TestSendRetriesInspectAfterDaemonSocketReset(t *testing.T) {
	home, err := os.MkdirTemp(os.Getenv("TMPDIR"), "s")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(home) }()
	listener, err := net.Listen("unix", filepath.Join(home, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	serveErr := make(chan error, 1)
	go func() {
		first, err := listener.Accept()
		if err != nil {
			serveErr <- err
			return
		}
		var request Request
		if err := json.NewDecoder(first).Decode(&request); err != nil {
			_ = first.Close()
			serveErr <- err
			return
		}
		_, _ = first.Write([]byte(`{"outcome":`))
		_ = first.Close()
		second, err := listener.Accept()
		if err != nil {
			serveErr <- err
			return
		}
		defer func() { _ = second.Close() }()
		if err := json.NewDecoder(second).Decode(&request); err != nil {
			serveErr <- err
			return
		}
		serveErr <- json.NewEncoder(second).Encode(Response{Version: 1, Outcome: "ok"})
	}()
	got, err := Send(home, Request{Version: 1, Op: "inspect"}, false)
	if err != nil || got.Outcome != "ok" {
		t.Fatalf("inspect after daemon restart = %+v, %v", got, err)
	}
	if err := <-serveErr; err != nil {
		t.Fatal(err)
	}
}

func TestStoppedDaemonCanRestartWithPendingRequest(t *testing.T) {
	bin, repo, home := fixture(t)
	axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	data, err := os.ReadFile(filepath.Join(home, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := json.Unmarshal(data, &pid); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unix", filepath.Join(home, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }() // Best-effort test connection cleanup.
	if _, err := conn.Write([]byte(`{"version":`)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
		if err != nil {
			t.Fatal(err)
		}
		sockets := 0
		for _, entry := range entries {
			target, _ := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, entry.Name()))
			if strings.HasPrefix(target, "socket:[") {
				sockets++
			}
		}
		if sockets >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not accept pending request")
		}
		time.Sleep(time.Millisecond)
	}
	if err := daemonutil.KillDaemon(home); err != nil {
		t.Fatal(err)
	}
	v := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	if v["outcome"] != "ok" {
		t.Fatalf("restart failed: %v", v)
	}
}

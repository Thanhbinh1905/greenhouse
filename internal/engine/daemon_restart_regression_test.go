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

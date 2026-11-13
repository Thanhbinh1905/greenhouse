package engine

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStopDaemon(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	daemon := exec.Command(bin, "daemon")
	daemon.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- daemon.Wait() }()
	t.Cleanup(func() { daemon.Process.Kill() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := Send(home, Request{Version: 2, Op: "attach", Path: repo}, false); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop := func(want string) {
		t.Helper()
		c := exec.Command(bin, "stop")
		c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
		b, err := c.CombinedOutput()
		if err != nil || !strings.Contains(string(b), want) {
			t.Fatalf("stop: %s %v; want %s", b, err, want)
		}
	}
	stop("daemon stopped")
	select {
	case err := <-exited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("daemon did not exit")
	}
	if _, err := os.Stat(filepath.Join(home, "pid")); !os.IsNotExist(err) {
		t.Fatalf("pid remains: %v", err)
	}
	stop("daemon is not running")
	if v := axi(t, bin, repo, home, `{"version":2,"op":"stop"}`); v["outcome"] != "ok" || v["explanation"] != "daemon is not running" {
		t.Fatalf("AXI absent: %v", v)
	}
	if _, err := os.Stat(filepath.Join(home, "pid")); !os.IsNotExist(err) {
		t.Fatalf("stop spawned daemon: %v", err)
	}
}

func TestStopRefusesBusyDaemon(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	release := filepath.Join(filepath.Dir(home), "release-review")
	installFake(t, home, "while not os.path.exists("+strconv.Quote(release)+"): time.sleep(0.02)")
	v = mutate(t, bin, repo, home, v, "review", "review-busy-stop", nil)
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	c := exec.Command(bin, "stop")
	c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
	b, err := c.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "daemon is busy") {
		t.Fatalf("busy CLI stop: %s %v", b, err)
	}
	refused := axi(t, bin, repo, home, `{"version":2,"op":"stop","host":{"name":"fake-host","id":"task-7"}}`)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "daemon is busy") {
		t.Fatalf("busy AXI stop: %v", refused)
	}
	if _, err := os.Stat(filepath.Join(home, "pid")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	settled := waitReview(t, bin, repo, home)
	if runOf(t, settled)["status"] != "outstanding-findings" {
		t.Fatalf("stop interrupted review: %v", settled)
	}
	stopped := axi(t, bin, repo, home, `{"version":2,"op":"stop"}`)
	if stopped["outcome"] != "ok" || stopped["explanation"] != "daemon stopped" {
		t.Fatalf("AXI stop: %v", stopped)
	}
	if _, err := os.Stat(filepath.Join(home, "pid")); !os.IsNotExist(err) {
		t.Fatalf("AXI stop left pid: %v", err)
	}
}

func TestStopUnsupportedDaemon(t *testing.T) {
	bin, repo, home := fixture(t)
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	defer signal.Stop(signals)
	for _, reply := range []Response{
		{Version: 1, Outcome: "refused", Error: "unsupported operation: stop"},
		{Version: 1, Outcome: "refused", Error: "unsupported protocol version"},
		{Version: 1, Outcome: "refused", Error: "unsupported protocol version 2; this daemon supports 1-1", Protocol: &Protocol{Version: 1, Min: 1, Max: 1}},
	} {
		t.Run(reply.Error, func(t *testing.T) {
			listener, err := net.Listen("unix", filepath.Join(home, "socket"))
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			if err := os.WriteFile(filepath.Join(home, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(filepath.Join(home, "pid"))
			finished := make(chan error, 1)
			go func() {
				c, err := listener.Accept()
				if err != nil {
					finished <- err
					return
				}
				defer c.Close()
				var q Request
				if err := json.NewDecoder(c).Decode(&q); err != nil {
					finished <- err
					return
				}
				finished <- json.NewEncoder(c).Encode(reply)
			}()
			c := exec.Command(bin, "stop")
			c.Dir = repo
			c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
			b, err := c.CombinedOutput()
			want := "let its work settle, then stop it with SIGTERM to the PID in GREENHOUSE_HOME/pid after confirming that process is the Greenhouse daemon"
			if err == nil || !strings.Contains(string(b), want) {
				t.Errorf("legacy guidance: %s %v", b, err)
			}
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			select {
			case sig := <-signals:
				t.Fatalf("stop signalled process: %v", sig)
			default:
			}
			if _, err := os.Stat(filepath.Join(home, "pid")); err != nil {
				t.Fatalf("stop removed legacy pid: %v", err)
			}
		})
	}
}

package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

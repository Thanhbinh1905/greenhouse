package engine

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestStopLegacyDaemon(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			bin, repo, home := fixture(t)
			t.Cleanup(func() { os.Remove(filepath.Join(home, "pid")) })
			_, source, _, _ := runtime.Caller(0)
			mainPath := filepath.Join(filepath.Dir(source), "main.go")
			content, err := os.ReadFile(mainPath)
			if err != nil {
				t.Fatal(err)
			}
			legacySource := filepath.Join(filepath.Dir(home), "legacy-main.go")
			content = []byte(strings.Replace(string(content), `if e == nil && q.Op == "stop" {`, `if e == nil && q.Op == "disabled-stop" {`, 1))
			content = []byte(strings.Replace(string(content), `defer os.Remove(filepath.Join(h, "pid"))`, "", 1))
			if err := os.WriteFile(legacySource, content, 0600); err != nil {
				t.Fatal(err)
			}
			replacements := map[string]string{mainPath: legacySource}
			if version == 1 {
				protocolPath := filepath.Join(filepath.Dir(source), "protocol.go")
				content, err := os.ReadFile(protocolPath)
				if err != nil {
					t.Fatal(err)
				}
				legacyProtocol := filepath.Join(filepath.Dir(home), "legacy-protocol.go")
				if err := os.WriteFile(legacyProtocol, []byte(strings.Replace(string(content), "MaxVersion  = 2", "MaxVersion  = 1", 1)), 0600); err != nil {
					t.Fatal(err)
				}
				replacements[protocolPath] = legacyProtocol
			}
			overlay, _ := json.Marshal(map[string]any{"Replace": replacements})
			overlayPath := filepath.Join(filepath.Dir(home), "overlay.json")
			if err := os.WriteFile(overlayPath, overlay, 0600); err != nil {
				t.Fatal(err)
			}
			legacy := filepath.Join(filepath.Dir(home), "legacy", "greenhouse")
			build := exec.Command("go", "build", "-race", "-overlay", overlayPath, "-o", legacy, "../../cmd/greenhouse")
			if b, err := build.CombinedOutput(); err != nil {
				t.Fatalf("legacy build: %s %v", b, err)
			}
			if err := os.MkdirAll(home, 0700); err != nil {
				t.Fatal(err)
			}
			daemon := exec.Command(legacy, "daemon")
			daemon.Dir = repo
			daemon.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0", "PATH="+filepath.Join(filepath.Dir(home), "native-bin")+":"+os.Getenv("PATH"), "HOME="+filepath.Dir(home), "GREENHOUSE_TEST_PROVIDER_KEY=fixture-secret", "PI_CODING_AGENT_DIR="+filepath.Join(filepath.Dir(home), "native-agent"))
			if err := daemon.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan error, 1)
			go func() { exited <- daemon.Wait() }()
			t.Cleanup(func() { daemon.Process.Kill() })
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, err := Send(home, Request{Version: 1, Op: "attach", Path: repo}, false); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("legacy daemon did not start")
				}
				time.Sleep(20 * time.Millisecond)
			}
			pidPath := filepath.Join(home, "pid")
			originalPID, err := os.ReadFile(pidPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
				t.Fatal(err)
			}
			c := exec.Command(bin, "stop")
			c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
			if b, err := c.CombinedOutput(); err == nil || !strings.Contains(string(b), "pid file does not identify") {
				t.Fatalf("mismatched PID: %s %v", b, err)
			}
			if err := os.WriteFile(pidPath, originalPID, 0600); err != nil {
				t.Fatal(err)
			}
			if version == 2 {
				target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
				if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600); err != nil {
					t.Fatal(err)
				}
				git(t, repo, "add", "sample.txt")
				git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
				release := filepath.Join(filepath.Dir(home), "release-legacy-review")
				installFake(t, home, "while not os.path.exists("+strconv.Quote(release)+"): time.sleep(0.02)")
				v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
				v = mutate(t, bin, repo, home, v, "new", "legacy-new", nil)
				settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"}
				v = mutate(t, bin, repo, home, v, "apply", "legacy-settings", settings)
				q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": "none", "action_id": "legacy-prepare", "intent": "Review sample change", "target": target}
				b, _ := json.Marshal(q)
				v = axi(t, bin, repo, home, string(b))
				v = mutate(t, bin, repo, home, v, "resolve-rules", "legacy-rules", nil)
				v = mutate(t, bin, repo, home, v, "review", "legacy-review", nil)
				if v["outcome"] != "ok" {
					t.Fatal(v)
				}
				busy := exec.Command(bin, "stop")
				busy.Env = c.Env
				if b, err := busy.CombinedOutput(); err == nil || !strings.Contains(string(b), "daemon is busy") {
					t.Fatalf("busy legacy daemon: %s %v", b, err)
				}
				if err := os.WriteFile(release, nil, 0600); err != nil {
					t.Fatal(err)
				}
				settled := waitReview(t, bin, repo, home)
				if runOf(t, settled)["status"] != "outstanding-findings" {
					t.Fatalf("legacy review interrupted: %v", settled)
				}
			}
			c = exec.Command(bin, "stop")
			c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
			b, err := c.CombinedOutput()
			if err != nil || !strings.Contains(string(b), "daemon stopped") {
				t.Fatalf("legacy stop: %s %v", b, err)
			}
			select {
			case err := <-exited:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("legacy daemon did not exit")
			}
			if _, err := os.Stat(filepath.Join(home, "pid")); !os.IsNotExist(err) {
				t.Fatalf("legacy pid remains: %v", err)
			}
		})
	}
}

func TestStopRefusesUnverifiedLegacyProcess(t *testing.T) {
	bin, repo, home := fixture(t)
	t.Cleanup(func() { os.Remove(filepath.Join(home, "pid")) })
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(home, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.WriteFile(filepath.Join(home, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
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
		finished <- json.NewEncoder(c).Encode(Response{Outcome: "refused", Error: "unsupported operation: stop"})
	}()
	c := exec.Command(bin, "stop")
	c.Dir = repo
	c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
	b, err := c.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "not a Greenhouse executable") {
		t.Fatalf("unverified process: %s %v", b, err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

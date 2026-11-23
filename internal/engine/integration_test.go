package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/daemonutil"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "HOME="+dir)
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("git: %s %v", b, e)
	}
	return string(b)
}

var daemonCleanupHomes sync.Map
var fixtureBuildOnce sync.Once
var fixtureBuildDir string
var fixtureBuildBin string
var fixtureBuildErr error

func TestMain(m *testing.M) {
	if err := os.Setenv("GREENHOUSE_TEST_PARENT_PID", fmt.Sprint(os.Getpid())); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	if fixtureBuildDir != "" {
		if err := os.RemoveAll(fixtureBuildDir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

func sharedFixtureBinary(t *testing.T) string {
	t.Helper()
	fixtureBuildOnce.Do(func() {
		fixtureBuildDir, fixtureBuildErr = os.MkdirTemp("", "gh-bin-")
		if fixtureBuildErr != nil {
			return
		}
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			fixtureBuildErr = fmt.Errorf("cannot locate fixture source")
			return
		}
		command := filepath.Join(filepath.Dir(source), "..", "..", "cmd", "greenhouse")
		fixtureBuildBin = filepath.Join(fixtureBuildDir, "greenhouse")
		args := []string{"build", "-o", fixtureBuildBin}
		if raceEnabled {
			args = append(args, "-race")
		}
		c := exec.Command("go", append(args, command)...)
		if b, err := c.CombinedOutput(); err != nil {
			fixtureBuildErr = fmt.Errorf("build: %s %w", b, err)
		}
	})
	if fixtureBuildErr != nil {
		t.Fatal(fixtureBuildErr)
	}
	return fixtureBuildBin
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	filtered := env[:0]
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func registerDaemonCleanup(t *testing.T, home string) {
	t.Helper()
	if os.Getenv("GREENHOUSE_TEST_CRASH_DAEMON") == "1" {
		return
	}
	if _, loaded := daemonCleanupHomes.LoadOrStore(home, struct{}{}); loaded {
		return
	}
	t.Cleanup(func() {
		if err := daemonutil.StopDaemon(home); err != nil {
			t.Error(err)
		}
	})
}

func fixture(t *testing.T) (string, string, string) {
	t.Helper()
	root, err := os.MkdirTemp("", "gh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	bin := sharedFixtureBinary(t)
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-b", "main")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	home := filepath.Join(root, "state")
	nativeHome := filepath.Join(root, "native-agent")
	if err := os.Mkdir(nativeHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nativeHome, "models.json"), []byte(`{"providers":{"fixture":{"models":[{"id":"model","reasoning":true}]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	registerDaemonCleanup(t, home)
	return bin, repo, home
}
func axi(t *testing.T, bin, repo, home, request string, env ...string) map[string]any {
	t.Helper()
	registerDaemonCleanup(t, home)
	c := exec.Command(bin, "axi")
	c.Dir = repo
	c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "HOME="+filepath.Dir(home), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GREENHOUSE_TEST_PROVIDER_KEY=fixture-secret", "PI_CODING_AGENT_DIR="+filepath.Join(filepath.Dir(home), "native-agent"))
	c.Env = append(c.Env, env...)
	c.Env = append(withoutEnv(c.Env, "GREENHOUSE_TEST_PARENT_PID"), "GREENHOUSE_TEST_PARENT_PID="+fmt.Sprint(os.Getpid()))
	fixturePath := os.Getenv("PATH")
	for _, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			fixturePath = strings.TrimPrefix(entry, "PATH=")
		}
	}
	c.Env = append(c.Env, "PATH="+filepath.Join(filepath.Dir(home), "native-bin")+":"+fixturePath)

	// Race-enabled clients otherwise delay timed snapshots by one second at exit.
	c.Env = append(c.Env, "GORACE="+os.Getenv("GORACE")+" atexit_sleep_ms=0")
	c.Stdin = strings.NewReader(request)
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("axi: %v %s", e, b)
	}
	var r map[string]any
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatalf("json %s: %v", b, e)
	}
	return r
}
func waitProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) {
			return
		}
		if err == nil {
			fields := strings.Fields(string(b))
			if len(fields) > 2 && fields[2] == "Z" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d remained alive after cleanup", pid)
}

func TestDaemonExitsWhenTestProcessDies(t *testing.T) {
	if os.Getenv("GREENHOUSE_TEST_CRASH_DAEMON") == "1" {
		bin := os.Getenv("GREENHOUSE_TEST_DAEMON_BIN")
		cmd := exec.Command(bin, "axi")
		cmd.Dir = os.Getenv("GREENHOUSE_TEST_REPO")
		cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+os.Getenv("GREENHOUSE_HOME"))
		cmd.Stdin = strings.NewReader(`{"version":1,"op":"attach"}`)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("start daemon: %v: %s", err, output)
		}
		pid, err := os.ReadFile(filepath.Join(os.Getenv("GREENHOUSE_HOME"), "pid"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv("GREENHOUSE_TEST_PID_FILE"), pid, 0600); err != nil {
			t.Fatal(err)
		}
		panic("simulated test panic")
	}

	bin, repo, home := fixture(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestDaemonExitsWhenTestProcessDies$")
	pidFile := filepath.Join(filepath.Dir(home), "daemon-pid")
	cmd.Env = append(withoutEnv(os.Environ(), "GREENHOUSE_TEST_PARENT_PID"),
		"GREENHOUSE_TEST_CRASH_DAEMON=1",
		"GREENHOUSE_TEST_DAEMON_BIN="+bin,
		"GREENHOUSE_TEST_REPO="+repo,
		"GREENHOUSE_TEST_PID_FILE="+pidFile,
		"GREENHOUSE_HOME="+home,
	)
	if output, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(output), "simulated test panic") {
		t.Fatalf("helper did not panic as expected: %v: %s", err, output)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := json.Unmarshal(b, &pid); err != nil {
		t.Fatal(err)
	}
	waitProcessExit(t, pid)
}

func TestFixtureStopsDaemonOnCleanup(t *testing.T) {
	var pid int
	t.Run("daemon", func(t *testing.T) {
		bin, repo, home := fixture(t)
		axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
		b, err := os.ReadFile(filepath.Join(home, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &pid); err != nil {
			t.Fatal(err)
		}
	})
	waitProcessExit(t, pid)
}

func TestFixtureStopsSecondaryDaemonOnCleanup(t *testing.T) {
	bin, repo, home := fixture(t)
	secondaryHome := filepath.Join(filepath.Dir(home), "secondary-state")
	var pid int
	t.Run("daemon", func(t *testing.T) {
		axi(t, bin, repo, secondaryHome, `{"version":1,"op":"attach"}`)
		b, err := os.ReadFile(filepath.Join(secondaryHome, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &pid); err != nil {
			t.Fatal(err)
		}
	})
	waitProcessExit(t, pid)
}

func TestConcurrentAttach(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	var a, b map[string]any
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`) }()
	go func() { defer wg.Done(); b = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`) }()
	wg.Wait()
	if a["run_id"] == nil || a["run_id"] != b["run_id"] {
		t.Fatalf("duplicate runs: %v %v", a, b)
	}
}

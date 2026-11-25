package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestClientDeadlineUsesTestPackageBound(t *testing.T) {
	t.Setenv("GREENHOUSE_TEST_CLIENT_TIMEOUT", "8m")
	q := Request{WaitMS: 2_000}
	if got, want := clientDeadline(q), 8*time.Minute; got != want {
		t.Fatalf("client deadline = %s, want %s", got, want)
	}
}

func TestDaemonSignalShutdownReturnsNil(t *testing.T) {
	const helperEnv = "GREENHOUSE_TEST_SIGNAL_SHUTDOWN"
	if os.Getenv(helperEnv) != "1" {
		root, err := os.MkdirTemp(os.TempDir(), "sig-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(root) }) // Best-effort cleanup of the temporary daemon home.
		home := filepath.Join(root, "state")
		if err := os.MkdirAll(home, 0700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestDaemonSignalShutdownReturnsNil$")
		cmd.Env = append(withoutEnv(os.Environ(), "GREENHOUSE_TEST_PARENT_PID"), helperEnv+"=1", "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("signal shutdown failed: %v: %s", err, output)
		}
		return
	}

	home := os.Getenv("GREENHOUSE_HOME")
	done := make(chan error, 1)
	go func() { done <- Daemon(home) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := Send(home, Request{Version: 1, Op: "attach"}, false); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("daemon stopped before becoming ready: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("daemon returned an error after signal shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not stop after SIGTERM")
	}
}

func TestAutoStartedDaemonDoesNotInheritClientWorkingDirectory(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	clientDir := filepath.Join(filepath.Dir(home), "client")
	if err := os.Mkdir(clientDir, 0700); err != nil {
		t.Fatal(err)
	}
	stdinPath := filepath.Join(filepath.Dir(home), "client-stdin")
	stdoutPath := filepath.Join(filepath.Dir(home), "client-stdout")
	stderrPath := filepath.Join(filepath.Dir(home), "client-stderr")
	stdin, err := os.Create(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stdin.Close(); err != nil {
			t.Error(err)
		}
	})
	stdout, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stdout.Close(); err != nil {
			t.Error(err)
		}
	})
	stderr, err := os.Create(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stderr.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := stdin.WriteString(`{"version":1,"op":"attach","path":"` + repo + `"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "axi")
	cmd.Dir = clientDir
	cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+home)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("attach from temporary client directory: %v", err)
	}
	pidBytes, err := os.ReadFile(filepath.Join(home, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid := strings.TrimSpace(string(pidBytes))
	cwd, err := os.Readlink(filepath.Join("/proc", pid, "cwd"))
	if err != nil {
		t.Fatal(err)
	}
	if cwd == clientDir {
		t.Fatalf("auto-started daemon retained client cwd %s", cwd)
	}
	entries, err := os.ReadDir(filepath.Join("/proc", pid, "fd"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range []string{"0", "1", "2"} {
		target, err := os.Readlink(filepath.Join("/proc", pid, "fd", fd))
		if err != nil {
			t.Fatalf("read daemon fd %s: %v", fd, err)
		}
		if target != os.DevNull {
			t.Fatalf("daemon fd %s points to %s, want %s", fd, target, os.DevNull)
		}
	}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc", pid, "fd", entry.Name()))
		if err != nil {
			continue
		}
		if target == clientDir || strings.HasPrefix(target, clientDir+string(os.PathSeparator)) {
			t.Fatalf("auto-started daemon holds fd %s under client directory: %s", entry.Name(), target)
		}
		for _, callerFile := range []string{stdinPath, stdoutPath, stderrPath} {
			if target == callerFile {
				t.Fatalf("auto-started daemon inherited caller fd %s: %s", entry.Name(), target)
			}
		}
	}
}

func TestCLIHelp(t *testing.T) {
	bin, repo, home := fixture(t)
	for _, args := range [][]string{{"--help"}, {"-h"}, {"axi", "--help"}, {"axi", "-h"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+filepath.Join(home, "unused"), "GORACE=atexit_sleep_ms=0")
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stdin.Close() }() // Best-effort test cleanup.
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("help must exit zero without reading stdin: %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("help wrote stderr: %q", stderr.String())
			}
			for _, want := range []string{"Usage:", "interactive", "line", "axi", "stdin", "intent", "target", "merge-base", "grant", "review", "docs/embedding.md"} {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("help missing %q: %s", want, stdout.String())
				}
			}
			if _, err := os.Stat(home); !os.IsNotExist(err) {
				t.Fatalf("help must not initialize state: %v", err)
			}
		})
	}
}

func TestCLIAXIWithoutHelp(t *testing.T) {
	bin, repo, home := fixture(t)
	request := `{"version":2,"op":"attach","host":{"name":"example","id":"task-1"}}`
	first := axi(t, bin, repo, home, request)
	second := axi(t, bin, repo, home, request)
	if first["outcome"] != "ok" || first["run_id"] == nil || first["run_id"] != second["run_id"] || first["token"] != second["token"] {
		t.Fatalf("AXI attach did not round-trip: first=%v second=%v", first, second)
	}
	for _, input := range []struct{ request, message string }{{"", "EOF"}, {`{"help":true}`, `json: unknown field "help"`}} {
		cmd := exec.Command(bin, "axi")
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
		cmd.Stdin = strings.NewReader(input.request)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			t.Fatalf("invalid AXI request must exit 1: %v", err)
		}
		want, err := json.Marshal(Response{Version: 1, Outcome: "error", Error: input.message})
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, '\n')
		if !bytes.Equal(stdout.Bytes(), want) || stderr.Len() != 0 {
			t.Fatalf("AXI error bytes changed: stdout=%q stderr=%q want=%q", stdout.Bytes(), stderr.Bytes(), want)
		}
	}
}

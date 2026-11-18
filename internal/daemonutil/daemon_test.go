package daemonutil

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestStopDaemonWaitsForPidAfterLockAcquisition(t *testing.T) {
	home := t.TempDir()
	lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	started := make(chan error, 1)
	go func() { started <- StopDaemon(home) }()
	time.Sleep(50 * time.Millisecond)
	select {
	case err := <-started:
		t.Fatalf("cleanup returned before locked daemon wrote pid: %v", err)
	default:
	}
	if err := os.WriteFile(filepath.Join(home, "pid"), []byte("999999999"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("cleanup did not finish after the lock was released")
	}
}

func TestStopDaemonRejectsUnreadableEnviron(t *testing.T) {
	home := t.TempDir()
	procRoot := t.TempDir()
	cmd := exec.Command("yes", "daemon")
	cmd.Args = []string{filepath.Join(home, "greenhouse"), "daemon"}
	cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+home)
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	proc := filepath.Join(procRoot, fmt.Sprint(cmd.Process.Pid))
	if err := os.Mkdir(proc, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "cmdline"), []byte("/opt/greenhouse\x00daemon\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(home, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "pid"), []byte(fmt.Sprint(cmd.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	err = signalDaemonWith(home, syscall.SIGTERM, func(pid int) (int, error) { return unix.PidfdOpen(pid, 0) }, procRoot)
	if err == nil {
		t.Fatal("unreadable environ passed identity verification")
	}
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("process was signaled despite unreadable environ: %v", err)
	}
}

func TestStopDaemonRefusesWithoutPidfd(t *testing.T) {
	home := t.TempDir()
	cmd := exec.Command("yes", "daemon")
	cmd.Args = []string{filepath.Join(home, "greenhouse"), "daemon"}
	cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+home)
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(home, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.WriteFile(filepath.Join(home, "pid"), []byte(fmt.Sprint(cmd.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	err = signalDaemonWith(home, syscall.SIGTERM, func(int) (int, error) { return -1, syscall.ENOSYS }, "/proc")
	if !errors.Is(err, syscall.ENOSYS) {
		t.Fatalf("pidfd failure = %v, want ENOSYS", err)
	}
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("process was signaled without pidfd: %v", err)
	}
}

func TestWaitForLockReleaseAfterProcessExit(t *testing.T) {
	if os.Getenv("DAEMONUTIL_HELPER") == "lock-holder" {
		home := os.Getenv("GREENHOUSE_HOME")
		lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_RDWR, 0600)
		if err != nil {
			os.Exit(2)
		}
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(os.Getenv("DAEMONUTIL_READY"), nil, 0600); err != nil {
			os.Exit(4)
		}
		os.Exit(0)
	}
	home := t.TempDir()
	lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ready := filepath.Join(home, "ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestWaitForLockReleaseAfterProcessExit$")
	cmd.Env = append(os.Environ(), "DAEMONUTIL_HELPER=lock-holder", "DAEMONUTIL_READY="+ready, "GREENHOUSE_HOME="+home)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatal("lock holder did not start")
	}
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", cmd.Process.Pid))
		if err == nil {
			fields := strings.Fields(string(b))
			if len(fields) > 2 && fields[2] == "Z" {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", cmd.Process.Pid))
	if err != nil || len(strings.Fields(string(b))) < 3 || strings.Fields(string(b))[2] != "Z" {
		t.Fatalf("lock holder did not exit into zombie state: %s %v", b, err)
	}
	started := time.Now()
	if err := waitForLockRelease(lock, time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) >= time.Second {
		t.Fatal("waitForLockRelease waited for process reaping")
	}
}

func TestStopDaemonIgnoresUnrelatedPID(t *testing.T) {
	home := t.TempDir()
	otherHome := t.TempDir()
	cmd := exec.Command("yes", "daemon")
	cmd.Args = []string{filepath.Join(home, "greenhouse"), "daemon"}
	cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+otherHome)
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()
	time.Sleep(20 * time.Millisecond)
	lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	listener, err := net.Listen("unix", filepath.Join(home, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.WriteFile(filepath.Join(home, "pid"), []byte(fmt.Sprint(cmd.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := StopDaemon(home); err == nil {
		t.Fatal("unrelated process identity passed verification")
	}
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("unrelated process exited: %v", err)
	}
}

package daemonutil

import (
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
)

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
	if err := StopDaemon(home); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("unrelated process exited: %v", err)
	}
}

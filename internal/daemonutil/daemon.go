package daemonutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func StopDaemon(home string) error {
	b, err := os.ReadFile(filepath.Join(home, "pid"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var pid int
	if err := json.Unmarshal(b, &pid); err != nil {
		return err
	}
	if pid <= 0 {
		return fmt.Errorf("invalid daemon pid %d", pid)
	}
	lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_RDWR, 0600)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		return nil
	} else if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
		return err
	}
	if !ownsHome(pid, home) {
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	if err := waitForLockRelease(lock, time.Now().Add(5*time.Second)); err != nil {
		return fmt.Errorf("daemon %d: %w", pid, err)
	}
	return nil
}

func waitForLockRelease(lock *os.File, deadline time.Time) error {
	for time.Now().Before(deadline) {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("did not release its state lock")
}

func ownsHome(pid int, home string) bool {
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	args := bytes.Split(bytes.TrimSuffix(cmdline, []byte{0}), []byte{0})
	if len(args) != 2 || filepath.Base(string(args[0])) != "greenhouse" || string(args[1]) != "daemon" {
		return false
	}
	environ, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err == nil {
		want := "GREENHOUSE_HOME=" + home
		found := false
		for _, entry := range bytes.Split(environ, []byte{0}) {
			if string(entry) == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	conn, err := net.DialTimeout("unix", filepath.Join(home, "socket"), 50*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

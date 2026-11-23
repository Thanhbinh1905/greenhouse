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

	"golang.org/x/sys/unix"
)

func StopDaemon(home string) error {
	return signalDaemon(home, syscall.SIGTERM)
}

func KillDaemon(home string) error {
	return signalDaemon(home, syscall.SIGKILL)
}

func signalDaemon(home string, signal syscall.Signal) error {
	return signalDaemonWith(home, signal, func(pid int) (int, error) { return unix.PidfdOpen(pid, 0) }, "/proc")
}

func signalDaemonWith(home string, signal syscall.Signal, openPidfd func(int) (int, error), procRoot string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(filepath.Join(home, "pid"))
		if os.IsNotExist(err) {
			released, err := tryAcquireStateLock(home)
			if err != nil {
				return err
			}
			if released {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
		var pid int
		if err := json.Unmarshal(b, &pid); err != nil {
			released, lockErr := tryAcquireStateLock(home)
			if lockErr != nil {
				return lockErr
			}
			if released {
				return err
			}
			time.Sleep(10 * time.Millisecond)
			continue
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
		released, err := tryAcquireLock(lock)
		if err != nil {
			return errors.Join(err, lock.Close())
		}
		if released {
			return lock.Close()
		}
		pidfd, err := openPidfd(pid)
		if err != nil {
			return errors.Join(fmt.Errorf("open pidfd for daemon %d: %w", pid, err), lock.Close())
		}
		if !ownsHome(pid, home, procRoot) {
			closeErr := unix.Close(pidfd)
			return errors.Join(fmt.Errorf("daemon %d does not match state home; refusing to signal", pid), closeErr, lock.Close())
		}
		err = unix.PidfdSendSignal(pidfd, signal, nil, 0)
		closeErr := unix.Close(pidfd)
		if err != nil && !errors.Is(err, syscall.ESRCH) {
			return errors.Join(err, closeErr, lock.Close())
		}
		if err := waitForLockRelease(lock, deadline); err != nil {
			return errors.Join(fmt.Errorf("daemon %d: %w", pid, err), closeErr, lock.Close())
		}
		return errors.Join(closeErr, lock.Close())
	}
	return fmt.Errorf("daemon pid remained unavailable while state lock was held")
}

func tryAcquireStateLock(home string) (bool, error) {
	lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_RDWR, 0600)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = lock.Close() }() // Closing a released state lock is best-effort cleanup.
	return tryAcquireLock(lock)
}

func tryAcquireLock(lock *os.File) (bool, error) {
	err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

func waitForLockRelease(lock *os.File, deadline time.Time) error {
	for time.Now().Before(deadline) {
		released, err := tryAcquireLock(lock)
		if err != nil {
			return err
		}
		if released {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("did not release its state lock")
}

func ownsHome(pid int, home, procRoot string) bool {
	proc := filepath.Join(procRoot, fmt.Sprint(pid))
	cmdline, err := os.ReadFile(filepath.Join(proc, "cmdline"))
	if err != nil {
		return false
	}
	args := bytes.Split(bytes.TrimSuffix(cmdline, []byte{0}), []byte{0})
	if len(args) != 2 || filepath.Base(string(args[0])) != "greenhouse" || string(args[1]) != "daemon" {
		return false
	}
	environ, err := os.ReadFile(filepath.Join(proc, "environ"))
	if err != nil {
		return false
	}
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
	conn, err := net.DialTimeout("unix", filepath.Join(home, "socket"), 2*time.Second)
	if err != nil {
		return false
	}
	return conn.Close() == nil
}

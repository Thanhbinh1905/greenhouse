package engine

import (
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func waitDaemonStopped(h string, wait bool) error {
	f, err := os.OpenFile(filepath.Join(h, "lock"), os.O_RDWR, 0600)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		if !wait {
			return errors.New("daemon owns state home but is unavailable; retry stop when it is ready")
		}
		if time.Now().After(deadline) {
			return errors.New("daemon shutdown did not finish within 5 seconds")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TerminalMutation(v Response, op string) Request {
	return Request{Version: MaxVersion, Op: op, RunID: v.RunID, Input: v.Input, Gate: v.Gate, Expected: v.Token, Action: opaque(), Client: "terminal"}
}

func BackendName(backend string) string  { return backendNames[backend] }
func OutcomeLabel(outcome string) string { return outcomeLabels[outcome] }

func stopLegacyDaemon(h string, c net.Conn) error {
	raw, err := c.(*net.UnixConn).SyscallConn()
	if err != nil {
		return err
	}
	var peer *unix.Ucred
	var peerErr error
	if err := raw.Control(func(fd uintptr) { peer, peerErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }); err != nil {
		return err
	}
	if peerErr != nil {
		return peerErr
	}
	data, err := os.ReadFile(filepath.Join(h, "pid"))
	if err != nil {
		return fmt.Errorf("cannot verify legacy daemon PID: %w", err)
	}
	var pid int
	if json.Unmarshal(data, &pid) != nil || pid <= 0 || pid != int(peer.Pid) || peer.Uid != uint32(os.Getuid()) {
		return errors.New("refusing legacy shutdown: pid file does not identify the socket's daemon")
	}
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return fmt.Errorf("cannot pin legacy daemon process: %w", err)
	}
	defer unix.Close(fd)
	proc := fmt.Sprintf("/proc/%d/", pid)
	info, err := buildinfo.ReadFile(proc + "exe")
	if err != nil || info.Path != "github.com/Thanhbinh1905/greenhouse/cmd/greenhouse" {
		return errors.New("refusing legacy shutdown: socket process is not a Greenhouse executable")
	}
	args, err := os.ReadFile(proc + "cmdline")
	if err != nil {
		return err
	}
	argv := strings.Split(strings.TrimSuffix(string(args), "\x00"), "\x00")
	if len(argv) != 2 || argv[1] != "daemon" {
		return errors.New("refusing legacy shutdown: process is not running as the Greenhouse daemon")
	}
	state, err := os.ReadFile(filepath.Join(h, "state.json"))
	if err != nil {
		return err
	}
	var store Store
	if err := json.Unmarshal(state, &store); err != nil {
		return err
	}
	if store.Version != 1 {
		return errors.New("refusing legacy shutdown: unsupported state version")
	}
	for _, r := range store.Runs {
		if r.Status == "running" || r.Status == "writing" || r.Status == "cancelling" || r.Recovery != nil || r.Pause != nil && r.Pause.AutoResumeAt != "" {
			return errors.New("daemon is busy; let running work settle before stopping")
		}
	}
	if err := unix.PidfdSendSignal(fd, unix.SIGTERM, nil, 0); err != nil {
		return err
	}
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	deadline := time.Now().Add(5 * time.Second)
	for {
		remaining := time.Until(deadline).Milliseconds()
		if remaining <= 0 {
			return errors.New("legacy daemon did not exit within 5 seconds")
		}
		n, err := unix.Poll(poll, int(remaining))
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n > 0 {
			break
		}
	}
	lock, err := os.OpenFile(filepath.Join(h, "lock"), os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	current, err := os.ReadFile(filepath.Join(h, "pid"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if string(current) != string(data) {
		return errors.New("daemon PID changed during legacy shutdown; refusing cleanup")
	}
	return os.Remove(filepath.Join(h, "pid"))
}

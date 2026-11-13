package engine

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
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

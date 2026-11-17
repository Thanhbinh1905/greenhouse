package daemonutil

import (
	"encoding/json"
	"errors"
	"fmt"
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
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("daemon %d did not exit", pid)
}

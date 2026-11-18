package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ErrWriterTreeUnknown = errors.New("writer descendant cleanup could not be verified")

func init() {
	if len(os.Args) > 2 && os.Args[1] == "_run-child" {
		os.Exit(runChild(os.Args[2:]))
	}
}

func reapChildTree() bool {
	children := filepath.Join("/proc/self/task", fmt.Sprint(os.Getpid()), "children")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(children)
		if err != nil {
			return false
		}
		pids := strings.Fields(string(data))
		if len(pids) == 0 {
			return true
		}
		for _, child := range pids {
			if pid, err := strconv.Atoi(child); err == nil {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		var status syscall.WaitStatus
		for {
			pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
			if err != nil || pid <= 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

type processTimeoutKey struct{}

func WithProcessTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, processTimeoutKey{}, timeout)
}

func ProcessTimeout(ctx context.Context) time.Duration {
	if timeout, ok := ctx.Value(processTimeoutKey{}).(time.Duration); ok {
		return timeout
	}
	return DefaultTimeout
}

func ProcessContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, ProcessTimeout(ctx))
}

func RunProcess(ctx context.Context, dir, exe string, args []string, input []byte, env []string, stdout, stderr io.Writer) (resultCode int, resultErr error) {
	if observer, ok := ctx.Value(observerKey{}).(ProcessObserver); ok {
		var finish func(int, error, time.Duration)
		stdout, stderr, finish = observer(append([]string{exe}, args...), stdout, stderr)
		started := time.Now()
		defer func() { finish(resultCode, resultErr, time.Since(started)) }()
	}
	ctx, cancel := ProcessContext(ctx)
	defer cancel()
	self, err := os.Executable()
	if err != nil {
		return -1, err
	}
	expires, _ := ctx.Deadline()
	deadline := strconv.FormatInt(expires.Add(30*time.Second).Unix(), 10)
	cmd := exec.CommandContext(ctx, self, append([]string{"_run-child", nextExecutor(ctx), deadline, exe}, args...)...)
	cmd.Dir = dir
	cmd.WaitDelay = 5 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		signal := syscall.SIGTERM
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			signal = executorTimeoutSignal
		}
		return cmd.Process.Signal(signal)
	}
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	e := cmd.Run()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	if code == 254 || errors.Is(e, exec.ErrWaitDelay) {
		return code, ErrWriterTreeUnknown
	}
	if cmd.ProcessState != nil {
		if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return code, ErrWriterTreeUnknown
		}
	}
	if ctx.Err() != nil {
		return code, ctx.Err()
	}
	if code == exitDetachedOutput && e != nil {
		return code, ErrDetachedOutput
	}
	return code, e
}

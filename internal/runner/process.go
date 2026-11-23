package runner

import (
	"bytes"
	"context"
	"errors"
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

func processDescendants(root int) (map[int]byte, error) {
	descendants := make(map[int]byte)
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		tasks := filepath.Join("/proc", strconv.Itoa(pid), "task")
		threads, err := os.ReadDir(tasks)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		children := make(map[int]struct{})
		for _, thread := range threads {
			data, err := os.ReadFile(filepath.Join(tasks, thread.Name(), "children"))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			for _, field := range strings.Fields(string(data)) {
				child, err := strconv.Atoi(field)
				if err != nil {
					return nil, err
				}
				children[child] = struct{}{}
			}
		}
		for child := range children {
			if _, seen := descendants[child]; seen {
				continue
			}
			data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(child), "stat"))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			end := strings.LastIndexByte(string(data), ')')
			if end < 0 {
				return nil, errors.New("invalid process stat")
			}
			fields := strings.Fields(string(data[end+1:]))
			if len(fields) < 1 || len(fields[0]) != 1 {
				return nil, errors.New("invalid process stat fields")
			}
			descendants[child] = fields[0][0]
			queue = append(queue, child)
		}
	}
	return descendants, nil
}

func freezeChildTree() ([]int, bool) {
	deadline := time.Now().Add(30 * time.Second)
	known := make(map[int]byte)
	stableScans := 0
	var latest map[int]byte
	for time.Now().Before(deadline) {
		descendants, err := processDescendants(os.Getpid())
		if err != nil {
			return processIDs(latest, false), false
		}
		latest = descendants
		newProcess := false
		allStopped := true
		for pid, state := range descendants {
			if _, seen := known[pid]; !seen {
				newProcess = true
			}
			known[pid] = state
			if state != 'T' && state != 't' && state != 'Z' && state != 'X' {
				allStopped = false
				if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil && !errors.Is(err, syscall.ESRCH) {
					return processIDs(latest, false), false
				}
			}
		}
		if newProcess || !allStopped {
			stableScans = 0
		} else {
			stableScans++
		}
		if stableScans >= 2 {
			return processIDs(latest, true), true
		}
		time.Sleep(time.Millisecond)
	}
	return processIDs(latest, false), false
}

func processIDs(processes map[int]byte, stoppedOnly bool) []int {
	pids := make([]int, 0, len(processes))
	for pid, state := range processes {
		if state == 'Z' || state == 'X' || (stoppedOnly && state != 'T' && state != 't') {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

func reapChildTree() bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		descendants, err := processDescendants(os.Getpid())
		if err != nil {
			return false
		}
		if len(descendants) == 0 {
			var status syscall.WaitStatus
			for {
				pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
				if err != nil || pid <= 0 {
					break
				}
			}
			return true
		}
		for pid := range descendants {
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				return false
			}
		}
		var status syscall.WaitStatus
		for {
			pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
			if err != nil || pid <= 0 {
				break
			}
		}
		time.Sleep(time.Millisecond)
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
	cmd.WaitDelay = 35 * time.Second
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

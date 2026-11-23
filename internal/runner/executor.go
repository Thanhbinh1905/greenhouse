package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// An executor is one child process started through the `_run-child` runner.
// The runner holds `<id>.lease` with flock for as long as it or any descendant
// that kept the inherited descriptor lives, captures bounded output in
// `<id>.out` and `<id>.err`, and writes `<id>.json` only after it reaped its
// whole process tree. A restarted daemon reads these files instead of PIDs.
type ExecutorRecord struct {
	InputDigest string `json:"input_digest"`
	Exit        int    `json:"exit"`
	Stopped     bool   `json:"stopped"`
	StopReason  string `json:"stop_reason,omitempty"`
	Reaped      bool   `json:"reaped"`
	Stdout      int64  `json:"stdout_bytes"`
	Stderr      int64  `json:"stderr_bytes"`
}

const (
	StdoutLimit           = 4 * 1024 * 1024
	StderrLimit           = 65536
	DefaultTimeout        = 45 * time.Minute
	exitDetachedOutput    = 253
	executorTimeoutSignal = syscall.SIGUSR1
)

var ErrDetachedOutput = errors.New("a descendant kept the output open after the executor exited")

type executorSequence struct {
	base        string
	resolveHome func() (string, error)
	next        atomic.Int32
}

type executorKey struct{}

func WithExecutor(ctx context.Context, base string, resolveHome func() (string, error)) context.Context {
	return context.WithValue(ctx, executorKey{}, &executorSequence{base: base, resolveHome: resolveHome})
}

func executorDir(resolveHome func() (string, error)) (string, error) {
	h, err := resolveHome()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(h, "executors")
	return dir, os.MkdirAll(dir, 0700)
}

func nextExecutor(ctx context.Context) string {
	seq, _ := ctx.Value(executorKey{}).(*executorSequence)
	if seq == nil {
		return "-"
	}
	dir, err := executorDir(seq.resolveHome)
	if err != nil {
		return "-"
	}
	return filepath.Join(dir, seq.base+"."+strconv.Itoa(int(seq.next.Add(1)-1)))
}

type capturedOutput struct {
	file  *os.File
	limit int64
	total int64
	pass  io.Writer
}

func (c *capturedOutput) Write(p []byte) (int, error) {
	if room := c.limit - c.total; room > 0 {
		chunk := p
		if int64(len(chunk)) > room {
			chunk = chunk[:room]
		}
		if n, err := c.file.Write(chunk); err != nil {
			return n, err
		} else if n != len(chunk) {
			return n, io.ErrShortWrite
		}
	}
	c.total += int64(len(p))
	if c.pass != nil {
		if _, err := c.pass.Write(p); err != nil {
			c.pass = nil
		}
	}
	return len(p), nil
}

func writeFileDurably(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".record-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }() // Best-effort cleanup when the durable write fails.
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	return errors.Join(syncErr, closeErr)
}

func runChild(args []string) (result int) {
	if len(args) < 3 {
		return 127
	}
	record, deadlineText, args := args[0], args[1], args[2:]
	if _, _, errno := syscall.RawSyscall6(syscall.SYS_PRCTL, 36, 1, 0, 0, 0, 0); errno != 0 {
		return 254
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, executorTimeoutSignal)
	defer signal.Stop(signals)
	broken := make(chan os.Signal, 1)
	signal.Notify(broken, syscall.SIGPIPE)
	defer signal.Stop(broken)
	deadlineUnix, err := strconv.ParseInt(deadlineText, 10, 64)
	if err != nil {
		return 127
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 8*1024*1024))
	if err != nil {
		return 127
	}
	child := exec.Command(args[0], args[1:]...)
	child.Stdin = bytes.NewReader(input)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child.WaitDelay = 250 * time.Millisecond
	var out, errOut *capturedOutput
	if record != "-" {
		lease, err := os.OpenFile(record+".lease", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return 254
		}
		defer func() {
			if err := lease.Close(); err != nil {
				result = 254
			}
		}()
		if err = syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			return 254
		}
		child.ExtraFiles = []*os.File{lease}
		stdout, err := os.OpenFile(record+".out", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return 254
		}
		defer func() {
			if err := stdout.Close(); err != nil {
				result = 254
			}
		}()
		stderr, err := os.OpenFile(record+".err", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return 254
		}
		defer func() {
			if err := stderr.Close(); err != nil {
				result = 254
			}
		}()
		out = &capturedOutput{file: stdout, limit: StdoutLimit + 1, pass: os.Stdout}
		errOut = &capturedOutput{file: stderr, limit: StderrLimit + 1, pass: os.Stderr}
		child.Stdout, child.Stderr = out, errOut
	}
	if err := child.Start(); err != nil {
		return 127
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	deadline := time.NewTimer(time.Until(time.Unix(deadlineUnix, 0)))
	defer deadline.Stop()
	var childErr error
	stopped := false
	stoppedAt := time.Time{}
	stopReason := ""
	select {
	case childErr = <-done:
	case sig := <-signals:
		stopped, stoppedAt, stopReason = true, time.Now(), "cancelled"
		if sig == executorTimeoutSignal {
			stopReason = "timeout"
		}
	case <-deadline.C:
		stopped, stoppedAt, stopReason = true, time.Now(), "timeout"
	}
	killErr := error(nil)
	freezeComplete := true
	var frozen []int
	if stopped {
		if err := syscall.Kill(child.Process.Pid, syscall.SIGSTOP); err != nil && !errors.Is(err, syscall.ESRCH) {
			killErr = err
		}
		cleanupDeadline := stoppedAt.Add(cancellationFreezeTimeout)
		frozen, freezeComplete = freezeChildTree(cleanupDeadline)
		if !freezeComplete {
			if err := syscall.Kill(-child.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				killErr = errors.Join(killErr, err)
			}
		}
		for _, pid := range frozen {
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				killErr = errors.Join(killErr, err)
			}
		}
		if !freezeComplete {
			reapExitedChildren()
			return 254
		}
		select {
		case childErr = <-done:
		case <-time.After(time.Until(cleanupDeadline)):
			reapExitedChildren()
			return 254
		}
	} else if err := syscall.Kill(-child.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		killErr = err
	}
	reaped := reapChildTree()
	code := 0
	var exitErr *exec.ExitError
	switch {
	case errors.Is(childErr, exec.ErrWaitDelay):
		code = exitDetachedOutput
	case errors.As(childErr, &exitErr):
		code = exitErr.ExitCode()
	case childErr != nil:
		code = 127
	}
	if killErr != nil || !freezeComplete {
		return 254
	}
	if record != "-" {
		if err := errors.Join(out.file.Sync(), errOut.file.Sync()); err != nil {
			return 254
		}
		data, _ := json.Marshal(ExecutorRecord{InputDigest: BytesDigest(input), Exit: code, Stopped: stopped, StopReason: stopReason, Reaped: reaped, Stdout: out.total, Stderr: errOut.total})
		if writeFileDurably(record+".json", data) != nil {
			return 254
		}
	}
	if !reaped {
		return 254
	}
	return code
}

type ExecutorEvidence struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Exit   int    `json:"exit"`
	Detail string `json:"detail,omitempty"`
}

type ObservedExecutor struct {
	ExecutorEvidence
	Record ExecutorRecord
	Stdout []byte
	Stderr []byte
}

func leaseHeld(path string) bool {
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }() // Close releases the probe descriptor after ownership is established.
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return true
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return true
	}
	return false
}

func ObserveExecutors(base string, resolveHome func() (string, error)) []ObservedExecutor {
	dir, err := executorDir(resolveHome)
	if err != nil {
		return nil
	}
	leases, _ := filepath.Glob(filepath.Join(dir, base+".*.lease"))
	sort.Slice(leases, func(i, j int) bool { return executorIndex(leases[i]) < executorIndex(leases[j]) })
	var out []ObservedExecutor
	for _, lease := range leases {
		path := strings.TrimSuffix(lease, ".lease")
		o := ObservedExecutor{ExecutorEvidence: ExecutorEvidence{ID: filepath.Base(path), State: "unverified", Exit: -1}}
		if leaseHeld(lease) {
			o.State, o.Detail = "running", "a live process holds the executor lease"
			out = append(out, o)
			continue
		}
		data, err := os.ReadFile(path + ".json")
		if err != nil || json.Unmarshal(data, &o.Record) != nil {
			o.Detail = "the runner left no complete exit record"
			out = append(out, o)
			continue
		}
		o.Exit = o.Record.Exit
		switch {
		case !o.Record.Reaped:
			o.Detail = "the runner could not verify that its process tree was reaped"
		case o.Record.Stopped:
			o.State, o.Detail = "stopped", "the runner stopped the child before it finished"
			o.Stdout, _ = readBounded(path+".out", StdoutLimit+1)
			o.Stderr, _ = readBounded(path+".err", StderrLimit+1)
		default:
			o.State = "exited"
			o.Stdout, _ = readBounded(path+".out", StdoutLimit+1)
			o.Stderr, _ = readBounded(path+".err", StderrLimit+1)
		}
		out = append(out, o)
	}
	return out
}

func executorIndex(lease string) int {
	name := strings.TrimSuffix(filepath.Base(lease), ".lease")
	n, _ := strconv.Atoi(name[strings.LastIndex(name, ".")+1:])
	return n
}

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // Closing a read-only capture is best-effort after reading.
	return io.ReadAll(io.LimitReader(f, limit))
}

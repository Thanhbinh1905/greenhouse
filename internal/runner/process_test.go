package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func TestRecordedProcess(t *testing.T) {
	home := t.TempDir()
	calls := 0
	resolveHome := func() (string, error) { calls++; return home, nil }
	ctx := runner.WithExecutor(context.Background(), "probe", resolveHome)
	if calls != 0 {
		t.Fatal("home resolved before execution")
	}
	out, stderr := runner.NewBuffer(4), runner.NewBuffer(4)
	code, err := runner.RunProcess(ctx, home, "/bin/sh", []string{"-c", "cat; printf err >&2"}, []byte("data"), os.Environ(), out, stderr)
	if err != nil || code != 0 || out.String() != "data" || stderr.String() != "err" {
		t.Fatalf("process: %d %v %q %q", code, err, out.String(), stderr.String())
	}
	observed := runner.ObserveExecutors("probe", resolveHome)
	if calls != 2 || len(observed) != 1 {
		t.Fatalf("observation: calls=%d records=%v", calls, observed)
	}
	got := observed[0]
	if got.State != "exited" || !got.Record.Reaped || got.Record.InputDigest != runner.BytesDigest([]byte("data")) || string(got.Stdout) != "data" || string(got.Stderr) != "err" {
		t.Fatalf("record: %+v", got)
	}
	data, err := os.ReadFile(filepath.Join(home, "executors", "probe.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record runner.ExecutorRecord
	if err := json.Unmarshal(data, &record); err != nil || record != got.Record {
		t.Fatalf("durable record: %s %v", data, err)
	}
	out.Write([]byte("overflow"))
	if !out.Exceeded() || out.Len() != 4 || string(out.Bytes()) != "data" {
		t.Fatalf("bounded output: %q overflow=%t", out.Bytes(), out.Exceeded())
	}
}

func TestConfiguredProcessTimeoutStopsExecutor(t *testing.T) {
	home := t.TempDir()
	resolveHome := func() (string, error) { return home, nil }
	ctx := runner.WithProcessTimeout(context.Background(), time.Second)
	ctx = runner.WithExecutor(ctx, "timeout", resolveHome)
	out, stderr := runner.NewBuffer(1024), runner.NewBuffer(1024)
	_, err := runner.RunProcess(ctx, home, "/bin/sh", []string{"-c", "sleep 30"}, nil, os.Environ(), out, stderr)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v (%s)", err, stderr.String())
	}
	observed := runner.ObserveExecutors("timeout", resolveHome)
	if len(observed) != 1 || observed[0].State != "stopped" || !observed[0].Record.Reaped || observed[0].Record.StopReason != "timeout" {
		t.Fatalf("timed-out executor tree was not stopped and reaped: %+v", observed)
	}
}

func TestConfiguredProcessDeadlineExceedsTenMinutes(t *testing.T) {
	for _, timeout := range []time.Duration{45 * time.Minute, time.Hour} {
		t.Run(timeout.String(), func(t *testing.T) {
			ctx := runner.WithProcessTimeout(context.Background(), timeout)
			processCtx, cancel := runner.ProcessContext(ctx)
			defer cancel()
			expires, ok := processCtx.Deadline()
			if !ok || time.Until(expires) < timeout-time.Second {
				t.Fatalf("executor deadline capped below %s: %v", timeout, expires)
			}
			out, stderr := runner.NewBuffer(4096), runner.NewBuffer(1024)
			before := time.Now()
			_, err := runner.RunProcess(processCtx, t.TempDir(), "/bin/sh", []string{"-c", `cat /proc/$PPID/cmdline`}, nil, os.Environ(), out, stderr)
			if err != nil {
				t.Fatalf("process: %v (%s)", err, stderr.String())
			}
			args := strings.Split(out.String(), "\x00")
			if len(args) < 4 || args[1] != "_run-child" {
				t.Fatalf("unexpected executor argv: %q", args)
			}
			deadline, err := strconv.ParseInt(args[3], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			want := expires.Add(30 * time.Second)
			if delta := want.Sub(time.Unix(deadline, 0)); delta < 0 || delta >= time.Second {
				t.Fatalf("descendant deadline %v does not follow executor deadline %v", time.Unix(deadline, 0), want)
			}
			if time.Unix(deadline, 0).Sub(before) <= 10*time.Minute {
				t.Fatal("descendant still has ten-minute cap")
			}
		})
	}
}

func TestProcessDeadlineHonorsEarlierParent(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ctx, stop := runner.ProcessContext(runner.WithProcessTimeout(parent, time.Hour))
	defer stop()
	got, _ := ctx.Deadline()
	want, _ := parent.Deadline()
	if got != want {
		t.Fatalf("parent deadline ignored: got %v, want %v", got, want)
	}
}

func TestRecordedCancellationKeepsOutput(t *testing.T) {
	home := t.TempDir()
	resolveHome := func() (string, error) { return home, nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = runner.WithExecutor(ctx, "cancel", resolveHome)
	out, stderr := runner.NewBuffer(1024), runner.NewBuffer(1024)
	_, err := runner.RunProcess(ctx, home, "/bin/sh", []string{"-c", "printf partial; sleep 30"}, nil, os.Environ(), cancelOnOutput{cancel: cancel, output: out}, stderr)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	observed := runner.ObserveExecutors("cancel", resolveHome)
	if len(observed) != 1 || observed[0].State != "stopped" || observed[0].Record.StopReason != "cancelled" || string(observed[0].Stdout) != "partial" {
		t.Fatalf("cancelled executor lost reason or output: %+v", observed)
	}
}

type cancelOnOutput struct {
	cancel context.CancelFunc
	output *runner.Buffer
}

func (w cancelOnOutput) Write(p []byte) (int, error) {
	n, err := w.output.Write(p)
	w.cancel()
	return n, err
}

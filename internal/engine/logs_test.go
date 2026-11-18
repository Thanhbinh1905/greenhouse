package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func TestLogsPreserveCursorsAndRedactSplitSecrets(t *testing.T) {
	t.Setenv("GREENHOUSE_HOME", filepath.Join(t.TempDir(), "state"))
	t.Setenv("WORKER_SECRET", "a-secret-across-chunks")
	writer := &logWriter{run: "run", step: "test"}
	writer.Write([]byte("before a-secret-"))
	view, err := readLogs("run", Request{Step: "test", Full: true})
	if err != nil || view.Bytes != 0 {
		t.Fatalf("partial output exposed: %v %v", view, err)
	}
	writer.Write([]byte("across-chunks\n"))
	writer.Write([]byte(strings.Repeat("plain output\n", 10000)))
	zero := int64(0)
	view, err = readLogs("run", Request{Step: "test", Offset: &zero})
	if err != nil || view.Offset != 0 || view.Next != 65536 || !strings.HasPrefix(view.Content, "before [REDACTED:WORKER_SECRET]") {
		t.Fatalf("explicit zero cursor: %v %v", view, err)
	}
	tail, err := readLogs("run", Request{Step: "test"})
	if err != nil || tail.Offset <= 0 || len(tail.Content) != 16384 {
		t.Fatalf("tail: %v %v", tail, err)
	}
	appendLog("run", "test", []byte(strings.Repeat("x", transcriptLimit)))
	full, err := readLogs("run", Request{Step: "test", Full: true})
	if err != nil || !full.Truncated || full.Bytes != transcriptLimit {
		t.Fatalf("bounded retention: %d %v %v", full.Bytes, full.Truncated, err)
	}
	home, _ := Home()
	info, err := os.Stat(filepath.Join(home, "logs", "run", "test.log"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private log: %v %v", info, err)
	}
}

func TestStepCompletionClassifiesExecutorErrors(t *testing.T) {
	t.Setenv("GREENHOUSE_HOME", filepath.Join(t.TempDir(), "state"))
	for _, executionErr := range []error{nil, runner.ErrWriterTreeUnknown, context.Canceled, context.DeadlineExceeded} {
		var events []FeedEvent
		observer := processObserver("run", "test", func(event FeedEvent) { events = append(events, event) })
		_, _, finish := observer([]string{"fixture"}, io.Discard, io.Discard)
		finish(0, executionErr, time.Millisecond)
		want := "step-finished"
		if executionErr != nil {
			want = "step-failed"
		}
		last := events[len(events)-1]
		if last.Kind != want || last.ExitCode == nil || *last.ExitCode != 0 {
			t.Fatalf("executor error %v: kind=%s exit=%v, want %s with exit=0", executionErr, last.Kind, last.ExitCode, want)
		}
	}
}

func TestStandaloneLogFollowOnlyWaitsForActiveStep(t *testing.T) {
	for _, active := range []struct{ stage, step, status string }{
		{"independent-review", "review", "running"},
		{"selected-repair", "repair", "writing"},
		{"selected-repair", "repair", "cancelling"},
	} {
		run := &Run{Stage: active.stage, Status: active.status}
		for _, step := range []string{"review", "repair", "test", "docs", "lint", "push", "pr", "ci"} {
			if got, want := logsComplete(run, step), step != active.step; got != want {
				t.Errorf("%s during %s: complete=%t, want %t", step, active.stage, got, want)
			}
		}
	}
}

func TestSnapshotTailRedactsBeforeTruncation(t *testing.T) {
	t.Setenv("WORKER_SECRET", "a-secret-across-chunks")
	tail := &tailBuffer{limit: 32}
	writer := &logWriter{output: tail}
	writer.Write([]byte("a-secret-"))
	writer.Write([]byte("across-chunks\n"))
	writer.Write([]byte("safe output\n"))
	writer.flush()
	if strings.Contains(tail.String(), "across-chunks") || !strings.Contains(tail.String(), "safe output") {
		t.Fatalf("redaction must precede tail truncation: %q", tail.String())
	}
}

func TestRecoveredCommandsRedactSnapshotOutput(t *testing.T) {
	plan := &Pipeline{Plan: CommandPlan{Test: [][]string{{"test"}}}}
	result, complete := recordedCommands(plan, "test", []runner.ObservedExecutor{{ExecutorEvidence: runner.ExecutorEvidence{State: "exited"}, Stdout: []byte("token=abcdefgh1234567890\n")}})
	if !complete || strings.Contains(result.Commands[0].Output, "abcdefgh1234567890") || !strings.Contains(result.Commands[0].Output, "[REDACTED]") {
		t.Fatalf("recovered output is not redacted: %+v", result)
	}
}

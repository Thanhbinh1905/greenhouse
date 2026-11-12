package runner_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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

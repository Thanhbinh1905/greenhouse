package eval

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/daemonutil"
)

func TestMain(m *testing.M) {
	if cfg := os.Getenv(shimEnv); cfg != "" {
		os.Exit(runShim(cfg))
	}
	os.Exit(m.Run())
}

const fakePi = `#!/usr/bin/python3
import json, os, sys
if sys.argv[1:] == ["--version"]:
    print("0.0.0-fake")
    sys.exit(0)
if "--list-models" in sys.argv:
    print("provider model context")
    print("fake m1 128K")
    sys.exit(0)
b = json.load(sys.stdin)
if "--no-tools" in sys.argv:
    assert sys.argv[sys.argv.index("--model") + 1] == "fake/judge"
    v = {"expected": b["expected"][0]["id"], "verdict": "detects", "rationale": 'The finding says "SQL injection" at the query.'}
    print(json.dumps({"type": "message_end", "message": {"role": "assistant", "provider": "fake", "model": "judge", "stopReason": "stop", "usage": {"cost": {"total": 0.01}}, "content": [{"type": "text", "text": json.dumps(v)}]}}))
    sys.exit(0)
sys.stderr.write(json.dumps({"shim": "GREENHOUSE_EVAL_SHIM" in os.environ}) + "\n")
findings = []
if "orders/orders.go" in b["Paths"]:
    findings.append({"id": "x", "priority": "P0", "origin": "correctness", "file": "orders/orders.go", "start": 19, "end": 19,
                     "explanation": "SQL injection " + os.environ["EVAL_TEST_API_KEY"], "action": "use a placeholder", "rule": "", "head": b["Head"]})
v = {"head": b["Head"], "verdict": "findings" if findings else "clean", "coverage": b["Paths"], "findings": findings}
usage = {"input": 100, "output": 10, "cacheRead": 0, "cacheWrite": 0, "reasoning": 5, "cost": {"total": 0.25}}
print(json.dumps({"type": "message_end", "message": {"role": "assistant", "provider": "fake", "model": "m1", "stopReason": "stop", "thinkingLevel": "medium", "durationMs": 7, "usage": usage, "content": [{"type": "text", "text": json.dumps(v)}]}}))
print(json.dumps({"type": "agent_settled", "aborted": False}))
`

func TestRunLiveDrivesGreenhouseWithoutNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the greenhouse executable")
	}
	dir := t.TempDir()
	pi := filepath.Join(dir, "pi")
	if err := os.WriteFile(pi, []byte(fakePi), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EVAL_TEST_API_KEY", "sk-test-0123456789")
	out := filepath.Join(dir, "out")
	r, err := runLive(runOptions{Corpus: filepath.Join("testdata", "corpus"), Out: out, Model: "fake/m1", Pi: pi, Label: "fake", JudgeModel: "fake/judge", Cases: []string{"bug-sql-injection", "clean-extract-helper"}, Parallel: 1, Settle: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cases) != 2 || r.Cases[0].ID != "bug-sql-injection" {
		t.Fatalf("cases = %+v", r.Cases)
	}
	bug, clean := r.Cases[0], r.Cases[1]
	if bug.Status != "outstanding-findings" || bug.Score.Detected != 1 || bug.Score.PriorityCorrect != 1 || !bug.InputVerified || bug.ReportedModel != "m1" || bug.ThinkingLevel != "medium" {
		t.Fatalf("bug record = %+v", bug)
	}
	if clean.Status != "clean-review" || clean.Score.FalsePositives != 0 || !clean.InputVerified {
		t.Fatalf("clean record = %+v", clean)
	}
	if r.CostUSD != 0.5 || r.Summary.HighDetected != 1 || !r.Summary.BarMet || r.Config.PiVersion == "" || r.Config.CorpusDigest == "" {
		t.Fatalf("report = %+v", r)
	}
	for _, name := range []string{"report.json", "report.md", "adjudications.json", "cases/bug-sql-injection/input.json", "cases/bug-sql-injection/argv.json", "cases/bug-sql-injection/events.jsonl"} {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "sk-test-0123456789") {
			t.Fatalf("%s leaks a credential", name)
		}
	}
	a, err := loadAdjudications(out)
	if err != nil || a == nil || len(a.Records) != 1 || a.Records[0].Verdict != "detects" || a.Judge.Model != "fake/judge" || !strings.Contains(a.Judge.Prompt, "different-defect") {
		t.Fatalf("adjudications = %+v %v", a, err)
	}
	saved, err := readReport(out)
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadCorpus(filepath.Join("testdata", "corpus"))
	if err != nil {
		t.Fatal(err)
	}
	if err := rescoreReport(saved, c, a); err != nil || saved.Summary.HighDetected != 1 {
		t.Fatalf("published report must rescore from its records: %v %+v", err, saved.Summary)
	}
	var argv []string
	b, _ := os.ReadFile(filepath.Join(out, "cases/bug-sql-injection/argv.json"))
	if json.Unmarshal(b, &argv) != nil || !strings.Contains(strings.Join(argv, " "), "--tools read,grep,find,ls") {
		t.Fatalf("argv = %s", b)
	}
	var env struct {
		Shim bool `json:"shim"`
	}
	b, _ = os.ReadFile(filepath.Join(out, "cases/bug-sql-injection/stderr.log"))
	if json.Unmarshal(b, &env) != nil || env.Shim {
		t.Fatalf("pi environment = %s", b)
	}
}

func TestCompactEventsDropsStreamingDeltas(t *testing.T) {
	in := "{\"type\":\"message_start\"}\n{\"type\":\"message_update\",\"x\":1}\n{\"type\":\"message_end\"}\nnot json\n"
	if got := string(compactEvents([]byte(in))); got != "{\"type\":\"message_start\"}\n{\"type\":\"message_end\"}\nnot json\n" {
		t.Fatalf("compacted = %q", got)
	}
}

func TestShimRetainsBothReviewTurns(t *testing.T) {
	dir := t.TempDir()
	pi := filepath.Join(dir, "pi")
	if err := os.WriteFile(pi, []byte("#!/bin/sh\ncat >/dev/null\nprintf '{\"type\":\"message_end\",\"message\":{\"role\":\"assistant\",\"usage\":{\"input\":7,\"output\":2}}}\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := shimConfig{Pi: pi, Capture: dir}
	for i, input := range []string{"initial input", "finalization input"} {
		var args []string
		if i == 1 {
			args = []string{"--no-tools"}
		}
		if code, err := shim(cfg, args, strings.NewReader(input), io.Discard, io.Discard); err != nil || code != 0 {
			t.Fatalf("shim: %d %v", code, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if got := parseEvents(data).Usage; got.Input != 14 || got.Output != 4 {
		t.Fatalf("lost first turn usage: %+v", got)
	}
	input, _ := os.ReadFile(filepath.Join(dir, "input.json"))
	if string(input) != "initial input" {
		t.Fatalf("lost admitted input: %q", input)
	}
}

func TestShimCountsTimeBeforeInterruptedTurn(t *testing.T) {
	dir := t.TempDir()
	pi := filepath.Join(dir, "pi")
	if err := os.WriteFile(pi, []byte("#!/bin/sh\ncat >/dev/null\n"), 0700); err != nil {
		t.Fatal(err)
	}
	prior := shimTiming{Start: time.Now().Add(-time.Second)}
	data, _ := json.Marshal(prior)
	if err := os.WriteFile(filepath.Join(dir, "timing.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := shim(shimConfig{Pi: pi, Capture: dir}, []string{"--no-tools"}, strings.NewReader("final"), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "timing.json"))
	var timing shimTiming
	if err := json.Unmarshal(data, &timing); err != nil {
		t.Fatal(err)
	}
	if timing.Millis < 1000 {
		t.Fatalf("lost interrupted exploration time: %+v", timing)
	}
}

func failDaemonCleanup(home string) error {
	if err := daemonutil.StopDaemon(home); err != nil {
		return err
	}
	return errors.New("forced daemon cleanup failure")
}

func TestRunLiveExcludesScoreAfterCleanupFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the greenhouse executable")
	}
	dir := t.TempDir()
	pi := filepath.Join(dir, "pi")
	if err := os.WriteFile(pi, []byte(fakePi), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EVAL_TEST_API_KEY", "sk-test-0123456789")
	r, err := runLive(runOptions{Corpus: filepath.Join("testdata", "corpus"), Out: filepath.Join(dir, "out"), Model: "fake/m1", Pi: pi, JudgeModel: "fake/judge", Cases: []string{"bug-sql-injection", "clean-extract-helper"}, Parallel: 1, Settle: time.Minute, cleanup: failDaemonCleanup})
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range r.Cases {
		if rec.Status != "eval-error" || rec.Error != "forced daemon cleanup failure" || !rec.InputVerified {
			t.Fatalf("completed review with failed cleanup = %+v", rec)
		}
	}
	if r.Cases[0].Score.Detected != 1 || len(r.Cases[1].Findings) != 0 {
		t.Fatalf("fixture must complete successful reviews before cleanup: %+v", r.Cases)
	}
	if r.Summary.Cases != 0 || r.Summary.HighDetected != 0 || r.Summary.BarMet {
		t.Fatalf("summary credits failed cleanup: %+v", r.Summary)
	}
}

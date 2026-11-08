package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
b = json.load(sys.stdin)
if "--no-tools" in sys.argv:
    assert sys.argv[sys.argv.index("--model") + 1] == "fake/judge"
    v = {"expected": b["expected"][0]["id"], "verdict": "detects", "rationale": 'The finding says "SQL injection" at the query.'}
    print(json.dumps({"type": "message_end", "message": {"role": "assistant", "provider": "fake", "model": "judge", "stopReason": "stop", "usage": {"cost": {"total": 0.01}}, "content": [{"type": "text", "text": json.dumps(v)}]}}))
    sys.exit(0)
root = os.path.dirname(os.path.abspath(__file__))
with open(os.path.join(root, "pi-env.json"), "w") as f:
    json.dump({"shim": "GREENHOUSE_EVAL_SHIM" in os.environ, "models": open(os.path.join(os.environ["PI_CODING_AGENT_DIR"], "models.json")).read()}, f)
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
	r, err := runLive(runOptions{Corpus: filepath.Join("testdata", "corpus"), Out: out, Model: "fake/m1", Pi: pi, BaseURL: "http://127.0.0.1:9/v1", Label: "fake", JudgeModel: "fake/judge", Cases: []string{"bug-sql-injection", "clean-extract-helper"}, Parallel: 2, Settle: time.Minute})
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
		Shim   bool   `json:"shim"`
		Models string `json:"models"`
	}
	b, _ = os.ReadFile(filepath.Join(dir, "pi-env.json"))
	if json.Unmarshal(b, &env) != nil || env.Shim || env.Models != `{"providers":{"fake":{"baseUrl":"http://127.0.0.1:9/v1"}}}` {
		t.Fatalf("pi environment = %s", b)
	}
}

func TestCompactEventsDropsStreamingDeltas(t *testing.T) {
	in := "{\"type\":\"message_start\"}\n{\"type\":\"message_update\",\"x\":1}\n{\"type\":\"message_end\"}\nnot json\n"
	if got := string(compactEvents([]byte(in))); got != "{\"type\":\"message_start\"}\n{\"type\":\"message_end\"}\nnot json\n" {
		t.Fatalf("compacted = %q", got)
	}
}

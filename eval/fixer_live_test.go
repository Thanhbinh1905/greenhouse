package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeFixerPi = `#!/usr/bin/python3
import json, os, sys
if sys.argv[1:] == ["--version"]:
    print("0.0.0-fake")
    sys.exit(0)
b = json.load(sys.stdin)
assert "read,edit,write,grep,find,ls" in sys.argv, sys.argv
def edit(path, old, new):
    src = open(path).read()
    assert old in src, (path, old)
    open(path, "w").write(src.replace(old, new))
response = "fixed"
if os.path.exists("window/window.go"):
    edit("window/window.go", "i+k < len(xs)", "i+k <= len(xs)")
elif os.path.exists("stats/stats.go"):
    edit("stats/stats.go", "\treturn s.Total / s.Count\n", "\tif s.Count == 0 {\n\t\treturn 0\n\t}\n\treturn s.Total / s.Count\n")
    edit("stats/stats.go", "\t// loop over all samples\n", "")
else:
    response = "disputed"
v = {"responses": [{"finding": f["id"], "response": response, "reason": "fake " + os.environ["EVAL_TEST_API_KEY"]} for f in b["Selected"]]}
usage = {"input": 1000000, "output": 1000000, "cacheRead": 0, "cacheWrite": 0, "reasoning": 0, "cost": {"total": 0}}
print(json.dumps({"type": "message_end", "message": {"role": "assistant", "provider": "fake", "model": "m1", "stopReason": "stop", "thinkingLevel": "medium", "usage": usage, "content": [{"type": "text", "text": json.dumps(v)}]}}))
print(json.dumps({"type": "agent_settled", "aborted": False}))
`

func TestRunFixLiveDrivesGreenhouseRepairWithoutNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the greenhouse executable")
	}
	dir := t.TempDir()
	pi := filepath.Join(dir, "pi")
	if err := os.WriteFile(pi, []byte(fakeFixerPi), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EVAL_TEST_API_KEY", "sk-test-0123456789")
	out := filepath.Join(dir, "out")
	r, err := runFixLive(fixOptions{Corpus: filepath.Join("testdata", "fixer"), Out: out, Model: "fake/m1", Pi: pi, Label: "fake", Cases: []string{"fix-offbyone-window", "fix-empty-average", "dispute-guarded-division"}, Parallel: 1, Settle: 2 * time.Minute, Price: Price{Input: 0.2, Output: 1.2}})
	if err != nil {
		t.Fatal(err)
	}
	window, average, dispute := r.Cases[0], r.Cases[1], r.Cases[2]
	if window.Status != "fixed-not-independently-rereviewed" || !window.Score.Fixed || !window.InputVerified || window.RepairHead == "" || window.Usage.CostUSD != 1.4 || window.CostSource != "tokens at the configured price" {
		t.Fatalf("in-scope repair = %+v", window)
	}
	if average.Status != "fixed-not-independently-rereviewed" || average.Score.Fixed || average.Score.AcceptPassed != 1 || len(average.Score.ScopeViolations) != 1 || !strings.Contains(average.Score.ScopeViolations[0], "stats/stats.go") {
		t.Fatalf("a repair that also removes the unselected finding's comment must be a scope violation: %+v", average.Score)
	}
	if dispute.Status != "responded-no-change" || !dispute.Score.Fixed || dispute.Score.Response != "disputed" {
		t.Fatalf("dispute = %+v", dispute)
	}
	if r.Summary.Fixed != 2 || r.Summary.ScopeViolations != 1 || r.Summary.BarMet || r.Config.Checks[1][1] != "test" {
		t.Fatalf("summary = %+v", r.Summary)
	}
	for _, name := range []string{"report.json", "report.md", "cases/fix-offbyone-window/input.json", "cases/fix-offbyone-window/argv.json", "cases/fix-offbyone-window/events.jsonl"} {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "sk-test-0123456789") {
			t.Fatalf("%s leaks a credential", name)
		}
	}
}

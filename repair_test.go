package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func repairRequest(t *testing.T, bin, repo, home string, v map[string]any, op, action string, selected []string) map[string]any {
	t.Helper()
	q := map[string]any{"version": 1, "op": op, "run_id": v["run_id"], "input_id": v["input_id"], "gate_id": "none", "expected": v["token"], "action_id": action, "selected": selected}
	b, _ := json.Marshal(q)
	return axi(t, bin, repo, home, string(b))
}
func repairFixture(t *testing.T) (string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home, v := prepareFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "dirty"), []byte("preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	v = mutate(t, bin, repo, home, v, "review", "review", nil)
	v = waitReview(t, bin, repo, home)
	p := filepath.Join(filepath.Dir(home), "fake-fixer")
	script := `#!/usr/bin/python3
import json,sys,os
b=json.load(sys.stdin)
with open(os.path.dirname(__file__)+'/fix-invocations','a') as f: f.write('fix\n')
with open(os.path.dirname(__file__)+'/fix-input.json','w') as f: json.dump({'args':sys.argv[1:],'input':b},f)
with open('sample.txt','w') as f: f.write('repaired\n')
`
	if err := os.WriteFile(p, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{"executable": p, "model": "fixture/model", "name": "Test", "email": "test@example.invalid", "checks": [][]string{{"/usr/bin/python3", "-c", "from pathlib import Path; assert Path('sample.txt').read_text() == 'repaired\\n'"}}}
	b, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(home, "fixer.json"), b, 0600)
	return bin, repo, home, v
}
func selectedID(t *testing.T, v map[string]any) []string {
	t.Helper()
	return []string{runOf(t, v)["findings"].([]any)[0].(map[string]any)["id"].(string)}
}
func waitRepair(t *testing.T, bin, repo, home string) map[string]any {
	t.Helper()
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		v := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		r := runOf(t, v)
		if r["status"] != "writing" && r["status"] != "cancelling" {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("repair did not settle")
	return nil
}
func TestRepairFailurePreservesCandidateAndBudget(t *testing.T) {
	bin, repo, home, v := repairFixture(t)
	configPath := filepath.Join(home, "fixer.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg FixerConfig
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Checks = [][]string{{"/usr/bin/python3", "-c", "raise SystemExit(7)"}}
	data, _ = json.Marshal(cfg)
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	response := repairRequest(t, bin, repo, home, v, "repair", "failing-check", selectedID(t, v))
	if response["outcome"] != "ok" {
		t.Fatal(response)
	}
	done := waitRepair(t, bin, repo, home)
	run := runOf(t, done)
	repair := run["repair"].(map[string]any)
	if repair["status"] != "failed-check" || run["repair_used"] != float64(1) || repair["checks"].([]any)[0].(map[string]any)["exit"] != float64(7) {
		t.Fatal(done)
	}
	worktree := repair["worktree"].(string)
	if strings.TrimSpace(git(t, worktree, "rev-parse", "HEAD")) != repair["base"] || git(t, worktree, "status", "--porcelain") == "" {
		t.Fatal("failed candidate was committed or discarded")
	}
	if next := repairRequest(t, bin, repo, home, done, "repair", "retry-failed", selectedID(t, done)); next["outcome"] != "refused" {
		t.Fatal("failed repair budget was reset", next)
	}
}

func TestExecutableSelectedRepair(t *testing.T) {
	bin, repo, home, v := repairFixture(t)
	before := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	selected := selectedID(t, v)
	admitted := repairRequest(t, bin, repo, home, v, "repair", "fix", selected)
	if admitted["outcome"] != "ok" {
		t.Fatalf("selected repair refused: %v", admitted["error"])
	}
	done := waitRepair(t, bin, repo, home)
	r := runOf(t, done)
	if r["status"] != "fixed-not-independently-rereviewed" || r["repair_used"] != float64(1) {
		t.Fatal(done)
	}
	repair := r["repair"].(map[string]any)
	if repair["status"] != "fixed-not-independently-rereviewed" || repair["parent"] != before || len(repair["checks"].([]any)) != 1 || repair["checks"].([]any)[0].(map[string]any)["status"] != "passed" {
		t.Fatalf("missing commit or check evidence: %v", repair)
	}
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(home), "invocations")); err != nil || strings.Count(string(b), "invoked\n") != 1 {
		t.Fatalf("repair unexpectedly rereviewed: %q %v", b, err)
	}
	if strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")) != before {
		t.Fatal("repair changed source")
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "dirty")); string(b) != "preserved" {
		t.Fatal("dirty work lost")
	}
	refused := repairRequest(t, bin, repo, home, done, "handback", "handback-dirty", nil)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "dirty") {
		t.Fatalf("dirty source must refuse handback: %v", refused)
	}
	if b, err := os.ReadFile(filepath.Join(repo, "dirty")); err != nil || string(b) != "preserved" {
		t.Fatalf("handback refusal damaged source work: %q %v", b, err)
	}
	if err := os.Remove(filepath.Join(repo, "dirty")); err != nil {
		t.Fatal(err)
	}
	handed := repairRequest(t, bin, repo, home, refused, "handback", "handback-safe", nil)
	if handed["outcome"] != "ok" || strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")) != repair["head"] {
		t.Fatalf("safe handback failed: %v", handed)
	}
	if replay := repairRequest(t, bin, repo, home, v, "repair", "fix", selected); replay["outcome"] != "ok" {
		t.Fatal(replay)
	}
	if second := repairRequest(t, bin, repo, home, done, "repair", "second", selected); second["outcome"] != "refused" {
		t.Fatal("second repair admitted")
	}
	stopDaemon(t, home)
	done = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	if runOf(t, done)["candidate"] != r["candidate"] || runOf(t, done)["repair_used"] != float64(1) || runOf(t, done)["repair"].(map[string]any)["handback"] != "handed-back" {
		t.Fatal("restart lost custody")
	}
	count, _ := os.ReadFile(filepath.Join(filepath.Dir(home), "fix-invocations"))
	if strings.Count(string(count), "fix\n") != 1 {
		t.Fatal(string(count))
	}
}

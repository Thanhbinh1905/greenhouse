package main

import (
	"context"
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
func TestRepairRunnerStopsDescendants(t *testing.T) {
	root := os.Getenv("TMPDIR")
	marker := filepath.Join(root, "repair-descendant-marker")
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	pidfile := filepath.Join(root, "repair-descendant-pid")
	script := filepath.Join(root, "repair-parent.py")
	program := "import subprocess,sys,pathlib; child=subprocess.Popen([sys.executable,'-c',\"import time,pathlib; time.sleep(1); pathlib.Path('" + marker + "').write_text('alive')\"]); pathlib.Path('" + pidfile + "').write_text(str(child.pid))"
	if err := os.WriteFile(script, []byte("#!/usr/bin/python3\n"+program+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	code, log, runErr := runBounded(context.Background(), root, script, nil, nil, "")
	time.Sleep(1200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived writer completion and wrote marker: code=%d log=%s err=%v stat=%v", code, log, runErr, err)
	}
}

func TestRepairRunnerStopsDescendantsOnCancellation(t *testing.T) {
	root := os.Getenv("TMPDIR")
	marker := filepath.Join(root, "repair-cancel-marker")
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	script := filepath.Join(root, "repair-cancel.py")
	program := "import subprocess,sys,time; subprocess.Popen([sys.executable,'-c',\"import time,pathlib; time.sleep(1); pathlib.Path('" + marker + "').write_text('alive'); time.sleep(30)\"], start_new_session=True); time.sleep(30)"
	if err := os.WriteFile(script, []byte("#!/usr/bin/python3\n"+program+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, _, _ = runBounded(ctx, root, script, nil, nil, "")
	time.Sleep(1200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("cancelled descendant remained able to write: %v", err)
	}
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

func TestLinkedWorktreeCannotAdmitConcurrentRepair(t *testing.T) {
	bin, repo, home, source := repairFixture(t)
	admitted := repairRequest(t, bin, repo, home, source, "repair", "fix-main", selectedID(t, source))
	if admitted["outcome"] != "ok" {
		t.Fatal(admitted)
	}
	done := waitRepair(t, bin, repo, home)
	if runOf(t, done)["status"] != "fixed-not-independently-rereviewed" {
		t.Fatal(done)
	}
	linked := filepath.Join(filepath.Dir(home), "linked")
	git(t, repo, "worktree", "add", "-b", "repair-conflict", linked, "HEAD")
	attached := axi(t, bin, linked, home, `{"version":1,"op":"attach"}`)
	target := strings.TrimSpace(git(t, linked, "rev-parse", "HEAD^"))
	q := map[string]any{"version": 1, "op": "prepare", "run_id": attached["run_id"], "input_id": attached["input_id"], "expected": attached["token"], "gate_id": "none", "action_id": "prepare-linked", "intent": "Review linked worktree", "target": target, "path": linked}
	b, _ := json.Marshal(q)
	prepared := axi(t, bin, linked, home, string(b))
	prepared = mutate(t, bin, linked, home, prepared, "resolve-rules", "rules-linked", nil)
	prepared = mutate(t, bin, linked, home, prepared, "review", "review-linked", nil)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		prepared = axi(t, bin, linked, home, `{"version":1,"op":"inspect"}`)
		if runOf(t, prepared)["status"] != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if runOf(t, prepared)["status"] != "outstanding-findings" {
		t.Fatalf("linked review did not complete with findings: %v", prepared)
	}
	conflict := repairRequest(t, bin, linked, home, prepared, "repair", "fix-linked", selectedID(t, prepared))
	if conflict["outcome"] != "refused" || !strings.Contains(conflict["error"].(string), "custody") {
		t.Fatalf("linked worktree admitted a competing repair: %v", conflict)
	}
}

func TestHandbackRefusesMovedSource(t *testing.T) {
	bin, repo, home, v := repairFixture(t)
	response := repairRequest(t, bin, repo, home, v, "repair", "fix-moved-source", selectedID(t, v))
	if response["outcome"] != "ok" {
		t.Fatal(response)
	}
	done := waitRepair(t, bin, repo, home)
	r := runOf(t, done)
	fix := r["repair"].(map[string]any)
	if err := os.Remove(filepath.Join(repo, "dirty")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "source-change"), []byte("user commit"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "source-change")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "user movement")
	sourceHead := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	moved := repairRequest(t, bin, repo, home, done, "handback", "handback-moved-source", nil)
	if moved["outcome"] != "refused" {
		t.Fatalf("moved source did not refuse: %v", moved)
	}
	if strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")) != sourceHead {
		t.Fatal("source head changed unexpectedly")
	}
	if strings.TrimSpace(git(t, fix["worktree"].(string), "rev-parse", "HEAD")) != fix["head"] {
		t.Fatal("candidate recovery commit was lost")
	}
}

func TestExecutableSelectedRepair(t *testing.T) {
	bin, repo, home, v := repairFixture(t)
	before := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	selected := selectedID(t, v)
	stale := repairRequest(t, bin, repo, home, v, "repair", "fix-stale", []string{"stale-finding"})
	if stale["outcome"] != "refused" || runOf(t, stale)["repair_used"] != float64(0) {
		t.Fatalf("stale selection consumed repair authority: %v", stale)
	}
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
	candidate := repair["worktree"].(string)
	if err := os.WriteFile(filepath.Join(candidate, "external-change"), []byte("outside writer"), 0600); err != nil {
		t.Fatal(err)
	}
	changed := repairRequest(t, bin, repo, home, refused, "handback", "handback-candidate-dirty", nil)
	if changed["outcome"] != "refused" {
		t.Fatalf("dirty candidate must refuse handback: %v", changed)
	}
	if err := os.Remove(filepath.Join(candidate, "external-change")); err != nil {
		t.Fatal(err)
	}
	handed := repairRequest(t, bin, repo, home, changed, "handback", "handback-safe", nil)
	if handed["outcome"] != "ok" || strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")) != repair["head"] {
		t.Fatalf("safe handback failed: %v", handed)
	}
	stopDaemon(t, home)
	stateBytes, _ := os.ReadFile(filepath.Join(home, "state.json"))
	var store Store
	if err := json.Unmarshal(stateBytes, &store); err != nil {
		t.Fatal(err)
	}
	stored := store.Runs[handed["run_id"].(string)]
	stored.Repair.Handback = "handback-in-progress"
	stored.Status = "fixed-not-independently-rereviewed"
	persist(home, &store)
	reconciled := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	if reconciled["outcome"] != "ok" || runOf(t, reconciled)["repair"].(map[string]any)["handback"] != "handed-back" {
		t.Fatalf("already-applied handback did not reconcile: %v", reconciled)
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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
func killRepairDaemon(t *testing.T, home string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if err = json.Unmarshal(data, &pid); err != nil {
		t.Fatal(err)
	}
	if err = syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("unix", filepath.Join(home, "socket"), 20*time.Millisecond)
		if dialErr != nil {
			return
		}
		conn.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("killed daemon listener remained open")
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
func TestRepairRunnerDeathMarksWriterUnknown(t *testing.T) {
	root := os.Getenv("TMPDIR")
	marker := filepath.Join(root, "repair-killed-runner-marker")
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	script := filepath.Join(root, "repair-kill-runner.py")
	program := "import os,signal,subprocess,sys; subprocess.Popen([sys.executable,'-c',\"import pathlib,time; time.sleep(.4); pathlib.Path('" + marker + "').write_text('late descendant write')\"], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True); os.kill(os.getppid(), signal.SIGKILL)"
	if err := os.WriteFile(script, []byte("#!/usr/bin/python3\n"+program+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	_, _, err := runBounded(context.Background(), root, script, nil, nil, "")
	if !errors.Is(err, ErrWriterTreeUnknown) {
		t.Fatalf("killed runner did not preserve unknown writer outcome: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(marker); statErr == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fixture descendant did not reproduce the late write")
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

func TestKilledWriterRunnerParksRepairCustody(t *testing.T) {
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
	cfg.Executable = filepath.Join(filepath.Dir(home), "kill-runner.py")
	program := "import os,signal,subprocess,sys; subprocess.Popen([sys.executable,'-c',\"import pathlib,time; time.sleep(.4); pathlib.Path('sample.txt').write_text('late descendant write')\"], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True); os.kill(os.getppid(), signal.SIGKILL)"
	if err = os.WriteFile(cfg.Executable, []byte("#!/usr/bin/python3\n"+program+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	admitted := repairRequest(t, bin, repo, home, v, "repair", "kill-runner", selectedID(t, v))
	if admitted["outcome"] != "ok" {
		t.Fatal(admitted)
	}
	done := waitRepair(t, bin, repo, home)
	repair := runOf(t, done)["repair"].(map[string]any)
	if repair["status"] != "unknown-outcome" || repair["writer"] != "unknown" {
		t.Fatalf("killed writer runner did not retain ambiguous custody: %v", repair)
	}
	worktree := repair["worktree"].(string)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		content, readErr := os.ReadFile(filepath.Join(worktree, "sample.txt"))
		if readErr == nil && string(content) == "late descendant write" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("detached fixture descendant did not reproduce the late candidate write")
}

func TestLiveFixerCancellationStopsDescendants(t *testing.T) {
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
	cfg.Executable = filepath.Join(filepath.Dir(home), "slow-fixer.py")
	started := filepath.Join(filepath.Dir(home), "fixer-started")
	program := "import pathlib,subprocess,sys,time; subprocess.Popen([sys.executable,'-c',\"import pathlib,time; time.sleep(4); pathlib.Path('sample.txt').write_text('late fixer write')\"], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True); pathlib.Path('" + started + "').write_text('started'); time.sleep(30)"
	if err = os.WriteFile(cfg.Executable, []byte("#!/usr/bin/python3\n"+program+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(cfg)
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	admitted := repairRequest(t, bin, repo, home, v, "repair", "slow-fixer", selectedID(t, v))
	if admitted["outcome"] != "ok" {
		t.Fatal(admitted)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(started); statErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err = os.Stat(started); err != nil {
		t.Fatal("fixer did not start")
	}
	current := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	cancelResponse := repairRequest(t, bin, repo, home, current, "cancel", "cancel-fixer", nil)
	if cancelResponse["outcome"] != "ok" {
		t.Fatal(cancelResponse)
	}
	done := waitRepair(t, bin, repo, home)
	repair := runOf(t, done)["repair"].(map[string]any)
	time.Sleep(4300 * time.Millisecond)
	content, err := os.ReadFile(filepath.Join(repair["worktree"].(string), "sample.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "review me\n" && (repair["status"] != "unknown-outcome" || repair["writer"] != "unknown") {
		t.Fatalf("live fixer cancellation claimed stopped writer after late candidate write: repair=%v response=%v content=%q", repair, cancelResponse, content)
	}
	if repair["status"] != "cancelled" && repair["status"] != "unknown-outcome" {
		t.Fatalf("live fixer cancellation did not settle safely: %v", repair)
	}
}

func TestLiveCheckCancellationStopsDescendants(t *testing.T) {
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
	checkScript := filepath.Join(filepath.Dir(home), "slow-check.py")
	checkStarted := filepath.Join(filepath.Dir(home), "check-started")
	checkProgram := `#!/usr/bin/python3
import pathlib,subprocess,sys,time
subprocess.Popen([sys.executable, "-c", "import pathlib,time; time.sleep(4); pathlib.Path('sample.txt').write_text('late check write')"], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
pathlib.Path("` + checkStarted + `").write_text("started")
time.sleep(30)
`
	if err = os.WriteFile(checkScript, []byte(checkProgram), 0700); err != nil {
		t.Fatal(err)
	}
	cfg.Checks = [][]string{{checkScript}}
	data, _ = json.Marshal(cfg)
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	admitted := repairRequest(t, bin, repo, home, v, "repair", "slow-check", selectedID(t, v))
	if admitted["outcome"] != "ok" {
		t.Fatal(admitted)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(checkStarted); statErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err = os.Stat(checkStarted); err != nil {
		t.Fatal("check did not start")
	}
	current := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	cancelResponse := repairRequest(t, bin, repo, home, current, "cancel", "cancel-check", nil)
	if cancelResponse["outcome"] != "ok" {
		t.Fatal(cancelResponse)
	}
	done := waitRepair(t, bin, repo, home)
	repair := runOf(t, done)["repair"].(map[string]any)
	checks := repair["checks"].([]any)
	time.Sleep(4300 * time.Millisecond)
	content, err := os.ReadFile(filepath.Join(repair["worktree"].(string), "sample.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "repaired\n" && (repair["status"] != "unknown-outcome" || repair["writer"] != "unknown" || checks[0].(map[string]any)["status"] != "unknown-outcome") {
		t.Fatalf("live check cancellation claimed stopped writer after late candidate write: repair=%v response=%v content=%q", repair, cancelResponse, content)
	}
	if repair["status"] != "cancelled" && repair["status"] != "unknown-outcome" {
		t.Fatalf("live check cancellation did not settle safely: %v", repair)
	}
}

func TestDaemonCrashDuringFixParksUnknownWriter(t *testing.T) {
	bin, repo, home, v := repairFixture(t)
	started := filepath.Join(filepath.Dir(home), "crash-fixer-started")
	configPath := filepath.Join(home, "fixer.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg FixerConfig
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Executable = filepath.Join(filepath.Dir(home), "crash-fixer.py")
	program := "import pathlib,time; pathlib.Path('" + started + "').write_text('started'); time.sleep(1); pathlib.Path('sample.txt').write_text('late after daemon crash')"
	if err = os.WriteFile(cfg.Executable, []byte("#!/usr/bin/python3\n"+program+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(cfg)
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	admitted := repairRequest(t, bin, repo, home, v, "repair", "crash-fixer", selectedID(t, v))
	if admitted["outcome"] != "ok" {
		t.Fatal(admitted)
	}
	repair := runOf(t, admitted)["repair"].(map[string]any)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(started); statErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err = os.Stat(started); err != nil {
		t.Fatal("fixer did not start")
	}
	killRepairDaemon(t, home)
	recovered := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	r := runOf(t, recovered)
	x := r["repair"].(map[string]any)
	if r["status"] != "recovery-required" || x["status"] != "unknown-outcome" || x["writer"] != "unknown" {
		t.Fatalf("daemon restart did not park interrupted repair: %v", recovered)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		content, readErr := os.ReadFile(filepath.Join(repair["worktree"].(string), "sample.txt"))
		if readErr == nil && string(content) == "late after daemon crash" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("interrupted writer fixture did not finish its candidate edit")
}

func TestAcceptedCancellationAtCommitAdmissionPreventsCommit(t *testing.T) {
	_, repo, home, v := repairFixture(t)
	stopDaemon(t, home)
	t.Setenv("GREENHOUSE_HOME", home)
	t.Setenv("GREENHOUSE_TEST_PROVIDER_KEY", "fixture-secret")
	store, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	q := Request{Version: 1, Op: "repair", Path: repo, RunID: v["run_id"].(string), Input: v["input_id"].(string), Gate: "none", Expected: v["token"].(string), Action: "repair-before-commit", Selected: selectedID(t, v)}
	admitted := handle(store, q)
	if admitted.Outcome != "ok" {
		t.Fatal(admitted)
	}
	r := store.Runs[q.RunID]
	repair := *r.Repair
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executeRepair(ctx, &repair, r.Submitted, func(value *Repair) bool {
		if value.Status == "committing" {
			cancelRequest := Request{Version: 1, Op: "cancel", Path: repo, RunID: r.ID, Input: r.Input, Gate: "none", Expected: r.Token, Action: "cancel-before-commit"}
			if response := handle(store, cancelRequest); response.Outcome != "ok" {
				t.Errorf("cancellation was not accepted before commit admission: run=%s repair=%s response=%v", r.Status, r.Repair.Status, response)
				return false
			}
			cancel()
		}
		return acceptRepairProgress(r, value.ID, *value)
	})
	candidate := &repair
	if candidate.Status != "cancelled" || r.Status != "cancelled" {
		t.Fatalf("accepted cancellation did not settle before commit: run=%s repair=%s", r.Status, candidate.Status)
	}
	if strings.TrimSpace(git(t, candidate.Worktree, "rev-parse", "HEAD")) != candidate.Base {
		t.Fatal("accepted pre-commit cancellation created a local commit")
	}
	committing := *r
	committing.Status = "writing"
	committing.Repair = &Repair{ID: candidate.ID, Status: "committing"}
	store.Runs[r.ID] = &committing
	lateCancel := Request{Version: 1, Op: "cancel", Path: repo, RunID: r.ID, Input: r.Input, Gate: "none", Expected: r.Token, Action: "cancel-after-commit-admission"}
	if response := handle(store, lateCancel); response.Outcome != "refused" {
		t.Fatalf("cancellation remained available after commit admission: %v", response)
	}
	if git(t, candidate.Worktree, "status", "--porcelain") == "" {
		t.Fatal("cancelled candidate edits were discarded")
	}
	if strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")) != r.Submitted.Head {
		t.Fatal("cancellation changed the source branch")
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

func TestLinkedWorktreeSerializesConcurrentRepairs(t *testing.T) {
	bin, repo, home, source := repairFixture(t)
	linked := filepath.Join(filepath.Dir(home), "linked")
	git(t, repo, "worktree", "add", "-b", "repair-conflict", linked, "HEAD")
	otherHome := filepath.Join(filepath.Dir(home), "linked-state")
	attached := axi(t, bin, linked, otherHome, `{"version":1,"op":"attach"}`)
	installFake(t, otherHome, "")
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop", "no-comments"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	attached = mutate(t, bin, linked, otherHome, attached, "apply", "settings-linked", settings)
	target := strings.TrimSpace(git(t, linked, "rev-parse", "HEAD^"))
	q := map[string]any{"version": 1, "op": "prepare", "run_id": attached["run_id"], "input_id": attached["input_id"], "expected": attached["token"], "gate_id": "none", "action_id": "prepare-linked", "intent": "Review linked worktree", "target": target}
	b, _ := json.Marshal(q)
	prepared := axi(t, bin, linked, otherHome, string(b))
	prepared = mutate(t, bin, linked, otherHome, prepared, "resolve-rules", "rules-linked", nil)
	prepared = mutate(t, bin, linked, otherHome, prepared, "review", "review-linked", nil)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		prepared = axi(t, bin, linked, otherHome, `{"version":1,"op":"inspect"}`)
		if runOf(t, prepared)["status"] != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if runOf(t, prepared)["status"] != "outstanding-findings" {
		t.Fatalf("linked review did not complete with findings: %v", prepared)
	}
	writer := filepath.Join(filepath.Dir(home), "slow-shared-writer.py")
	writerLog := filepath.Join(filepath.Dir(home), "shared-writer-invocations")
	primaryConfig, err := os.ReadFile(filepath.Join(home, "fixer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(otherHome, "fixer.json"), primaryConfig, 0600); err != nil {
		t.Fatal(err)
	}
	program := "import pathlib,time; pathlib.Path('" + writerLog + "').open('a').write('write\\n'); time.sleep(2); pathlib.Path('sample.txt').write_text('repaired\\n')"
	if err := os.WriteFile(writer, []byte("#!/usr/bin/python3\n"+program+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, stateHome := range []string{home, otherHome} {
		configPath := filepath.Join(stateHome, "fixer.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		var cfg FixerConfig
		if err = json.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		cfg.Executable = writer
		data, _ = json.Marshal(cfg)
		if err = os.WriteFile(configPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	first := repairRequest(t, bin, repo, home, source, "repair", "fix-main", selectedID(t, source))
	if first["outcome"] != "ok" {
		t.Fatal(first)
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		current := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		if runOf(t, current)["repair"].(map[string]any)["status"] == "fixing" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	second := repairRequest(t, bin, linked, otherHome, prepared, "repair", "fix-linked", selectedID(t, prepared))
	if second["outcome"] != "ok" {
		t.Fatal(second)
	}
	var firstDone, secondDone map[string]any
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		firstDone = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		secondDone = axi(t, bin, linked, otherHome, `{"version":1,"op":"inspect"}`)
		firstRun, secondRun := runOf(t, firstDone), runOf(t, secondDone)
		firstStatus := firstRun["repair"].(map[string]any)["status"]
		secondStatus := secondRun["repair"].(map[string]any)["status"]
		busy := func(status string) bool {
			return status == "fixing" || status == "checking" || status == "admitted" || status == "running"
		}
		if firstRun["status"] != "writing" && secondRun["status"] != "writing" && !busy(firstStatus.(string)) && !busy(secondStatus.(string)) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	firstRepair := runOf(t, firstDone)["repair"].(map[string]any)
	secondRepair := runOf(t, secondDone)["repair"].(map[string]any)
	if firstRepair["status"] != "fixed-not-independently-rereviewed" || secondRepair["status"] != "writer-conflict" {
		t.Fatalf("linked worktree writers were not serialized: first=%v second=%v", firstRepair["status"], secondRepair["status"])
	}
	stopDaemon(t, otherHome)
	stopDaemon(t, home)
	invocations, err := os.ReadFile(writerLog)
	if err != nil || strings.Count(string(invocations), "write\n") != 1 {
		t.Fatalf("expected one writer in the shared repository: %q %v", invocations, err)
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
	staleAction := repairRequest(t, bin, repo, home, v, "repair", "fix-stale-token", selected)
	if staleAction["outcome"] != "refused" {
		t.Fatalf("stale repair token was accepted: %v", staleAction)
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
	committedHead := repair["head"].(string)
	killRepairDaemon(t, home)
	done = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	r = runOf(t, done)
	repair = r["repair"].(map[string]any)
	if r["status"] != "fixed-not-independently-rereviewed" || r["repair_used"] != float64(1) || repair["head"] != committedHead {
		t.Fatalf("restart lost committed repair evidence: %v", done)
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

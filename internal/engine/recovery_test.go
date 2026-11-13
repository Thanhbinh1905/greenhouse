package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

func copyStateHome(t *testing.T, from, to string) {
	t.Helper()
	if err := os.MkdirAll(to, 0700); err != nil {
		t.Fatal(err)
	}
	err := filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		if rel == "." || rel == "socket" || rel == "lock" || rel == "pid" {
			return nil
		}
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func readRefLease(t *testing.T, repo, ref string) *RefLease {
	t.Helper()
	common, err := filepath.EvalSymlinks(strings.TrimSpace(git(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))
	if err != nil {
		t.Fatal(err)
	}
	lease, err := readLease(common, ref)
	if err != nil {
		t.Fatal(err)
	}
	return lease
}

func TestRefLeaseSurvivesRestartAndRefusesOtherWriters(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "kill-test"), []byte("1"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := runOf(t, waitPipeline(t, bin, repo, home))
	pipe := parked["pipeline"].(map[string]any)
	ref := pipe["ref"].(string)
	if parked["status"] != "recovery-required" || pipe["lease_epoch"] != float64(1) {
		t.Fatalf("pipeline did not record its candidate lease: %v", pipe)
	}
	os.Remove(filepath.Join(root, "kill-test"))
	stopDaemonAndWait(t, home)
	lease := readRefLease(t, repo, ref)
	if lease == nil || lease.Epoch != 1 || lease.Home != home {
		t.Fatalf("lease did not survive the daemon: %v", lease)
	}
	candidate := strings.TrimSpace(git(t, pipe["worktree"].(string), "rev-parse", "HEAD"))
	backup, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	second := filepath.Join(root, "second")
	copyStateHome(t, home, second)
	other := waitProbed(t, bin, repo, second)
	refused := recoverRequest(t, bin, repo, second, other, "discard", "second-daemon-discard")
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "leased by another Greenhouse state home") {
		t.Fatalf("a second daemon wrote the leased candidate: %v", refused)
	}
	stopDaemonAndWait(t, second)

	current := waitProbed(t, bin, repo, home)
	os.WriteFile(filepath.Join(root, "kill-test"), []byte("1"), 0600)
	if discarded := recoverRequest(t, bin, repo, home, current, "discard", "owner-discard"); discarded["outcome"] != "ok" {
		t.Fatal(discarded)
	}
	if rerun := runOf(t, waitPipeline(t, bin, repo, home)); rerun["status"] != "recovery-required" || rerun["pipeline"].(map[string]any)["lease_epoch"] != float64(2) {
		t.Fatalf("discard did not rerun the stage under a renewed lease: %v", rerun["pipeline"])
	}
	if lease = readRefLease(t, repo, ref); lease == nil || lease.Epoch != 2 {
		t.Fatalf("discard did not renew the lease epoch: %v", lease)
	}
	stopDaemonAndWait(t, home)
	newer, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(home, "state.json"), backup, 0600); err != nil {
		t.Fatal(err)
	}
	restored := waitProbed(t, bin, repo, home)
	stale := recoverRequest(t, bin, repo, home, restored, "discard", "stale-discard")
	if stale["outcome"] != "refused" || !strings.Contains(stale["error"].(string), "stale lease") {
		t.Fatalf("a stale store wrote the candidate: %v", stale)
	}
	if got := strings.TrimSpace(git(t, pipe["worktree"].(string), "rev-parse", "HEAD")); got != candidate {
		t.Fatalf("refused writers moved the candidate: %s", got)
	}
	if lease = readRefLease(t, repo, ref); lease == nil || lease.Epoch != 2 {
		t.Fatalf("a stale store released the newer lease: %v", lease)
	}
	stopDaemonAndWait(t, home)

	if err = os.WriteFile(filepath.Join(home, "state.json"), newer, 0600); err != nil {
		t.Fatal(err)
	}
	foreignPath := filepath.Join(filepath.Dir(leasePathFor(t, repo, ref)), "foreign.json")
	data, _ := json.Marshal(RefLease{Ref: "refs/heads/greenhouse/foreign", Owner: "another-owner", Home: "/elsewhere", Epoch: 3})
	if err = os.WriteFile(foreignPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	latest := waitProbed(t, bin, repo, home)
	if next := recoverRequest(t, bin, repo, home, latest, "new-run", "release-new-run"); next["outcome"] != "ok" {
		t.Fatal(next)
	}
	if lease = readRefLease(t, repo, ref); lease != nil {
		t.Fatalf("the dead lease of a superseded run was kept: %v", lease)
	}
	if _, err = os.Stat(foreignPath); err != nil {
		t.Fatalf("another state home's lease was released: %v", err)
	}
}

func leasePathFor(t *testing.T, repo, ref string) string {
	t.Helper()
	common, err := filepath.EvalSymlinks(strings.TrimSpace(git(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))
	if err != nil {
		t.Fatal(err)
	}
	return leasePath(common, ref)
}

func TestAdoptedGateRepairCommitContinuesPipeline(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	stopDaemonAndWait(t, home)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(root, "commit-crash-bin")
	os.MkdirAll(wrapper, 0700)
	marker := filepath.Join(root, "kill-after-commit")
	os.WriteFile(marker, []byte("1"), 0600)
	writeExecutable(t, filepath.Join(wrapper, "git"), `#!/usr/bin/python3
import json,os,subprocess,sys
args=sys.argv[1:]
p=subprocess.run([`+quotePython(realGit)+`]+args)
if p.returncode == 0 and 'commit' in args and any('/candidates/' in a for a in args) and os.path.exists(`+quotePython(marker)+`):
    os.kill(json.load(open(os.path.join(os.environ['GREENHOUSE_HOME'],'pid'))),9)
    os.unlink(`+quotePython(marker)+`)
sys.exit(p.returncode)
`)
	path := "PATH=" + wrapper + string(os.PathListSeparator) + os.Getenv("PATH")
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	prepare := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": "prepare-wrapped", "intent": "Change the sample", "target": runOf(t, v)["target"]}
	b, _ := json.Marshal(prepare)
	if v = axi(t, bin, repo, home, string(b), path); v["outcome"] != "ok" {
		t.Fatal(v)
	}
	q := map[string]any{"version": 1, "op": "pipeline", "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": "start", "reason": "test"}
	b, _ = json.Marshal(q)
	if started := axi(t, bin, repo, home, string(b), path); started["outcome"] != "ok" {
		t.Fatal(started)
	}
	deadline := time.Now().Add(30 * time.Second)
	for _, err := os.Stat(marker); err == nil && time.Now().Before(deadline); _, err = os.Stat(marker) {
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("the repair never committed: %v", runOf(t, axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`, path))["pipeline"])
	}
	waitDaemonExit(t, home)
	var recovered map[string]any
	for time.Now().Before(deadline) {
		recovered = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`, path)
		if rec, ok := runOf(t, recovered)["recovery"].(map[string]any); ok && strings.Contains(fmt.Sprint(rec["options"]), "adopt") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	rec := runOf(t, recovered)["recovery"].(map[string]any)
	if rec["kind"] != "repair" || rec["stage"] != "test" || fmt.Sprint(rec["options"]) != "[inspect adopt discard new-run]" {
		t.Fatalf("interrupted gate repair commit is not adoptable: %v", rec)
	}
	head := rec["candidate"].(map[string]any)["observed"]
	q = map[string]any{"version": 1, "op": "recover", "run_id": recovered["run_id"], "input_id": recovered["input_id"], "gate_id": recovered["gate_id"], "expected": recovered["token"], "action_id": "adopt-gate-repair", "choice": "adopt", "reason": "commit matches the checked repair"}
	b, _ = json.Marshal(q)
	if adopted := axi(t, bin, repo, home, string(b), path); adopted["outcome"] != "ok" || runOf(t, adopted)["candidate"] != head {
		t.Fatalf("adopt did not record the repaired candidate: %v", adopted)
	}
	done := waitPipeline(t, bin, repo, home)
	pipe := runOf(t, done)["pipeline"].(map[string]any)
	if stageOf(t, done, "test")["status"] != "passed" || stageOf(t, done, "review")["candidate"] != head || pipe["repair_used"] != float64(1) {
		t.Fatalf("pipeline did not continue on the adopted candidate: %v", pipe)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["test"] != 1 {
		t.Fatalf("adoption reran the fixer: %v", got)
	}
}

func TestPipelineStartSurvivesSlowCandidateCreation(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	stopDaemonAndWait(t, home)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(filepath.Dir(home), "slow-git-bin")
	if err := os.MkdirAll(wrapper, 0700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(wrapper, "git"), `#!/usr/bin/python3
import os,sys,time
args=sys.argv[1:]
if 'worktree' in args and 'add' in args:
    time.sleep(5.1)
os.execv(`+quotePython(realGit)+`,[`+quotePython(realGit)+`]+args)
`)
	path := "PATH=" + wrapper + string(os.PathListSeparator) + os.Getenv("PATH")
	prepare := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": "prepare-slow-git", "intent": "Change the sample", "target": runOf(t, v)["target"]}
	b, _ := json.Marshal(prepare)
	v = axi(t, bin, repo, home, string(b), path)
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	started := mutate(t, bin, repo, home, v, "pipeline", "slow-start", nil)
	if started["outcome"] != "ok" {
		t.Fatalf("slow candidate creation lost the pipeline admission response: %v", started)
	}
	done := waitPipeline(t, bin, repo, home)
	if stageOf(t, done, "sync")["status"] != "passed" || stageOf(t, done, "test")["status"] != "passed" {
		t.Fatalf("slow candidate creation did not pass synchronization and tests: %v", done)
	}
}

func TestRecoveryNewRunCarriesHostIdentityAndSpentGrants(t *testing.T) {
	t.Parallel()
	source := repository.Identity{Repository: "/repo", Branch: "main", Head: "head"}
	host := &Host{Name: "fake-host", ID: "task-7"}
	r := &Run{
		ID: "run-1", Submitted: source, Host: host, HostGrants: []string{"grant-1", "grant-2"},
		ReviewUsed: 2, RepairUsed: 1,
		Configs:  []Snapshot{{ID: "config", Settings: Settings{Mode: "auto", Preset: "one-pass", Limit: 4}}},
		Recovery: &Recovery{Kind: "run", Options: []string{"new-run"}},
	}
	s := &Store{Runs: map[string]*Run{r.ID: r}}
	next, err := recoverRun(s, r, source, Answer{Choice: "new-run", Reason: "resume under host ownership"})
	if err != nil {
		t.Fatal(err)
	}
	if next.Host == nil || !sameHost(next.Host, host) {
		t.Fatalf("recovery successor lost host identity: %+v", next.Host)
	}
	if next.ReviewUsed != r.ReviewUsed || next.RepairUsed != r.RepairUsed {
		t.Fatalf("recovery successor reset spent grants: review=%d repair=%d", next.ReviewUsed, next.RepairUsed)
	}
	if !slices.Equal(next.HostGrants, r.HostGrants) {
		t.Fatalf("recovery successor grants = %v, want %v", next.HostGrants, r.HostGrants)
	}
}

func TestRecoveryNewRunKeepsFinalFixGrantAcrossPrepare(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := authorityFixture(t, "auto", 1, repairPlan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("clean"), 0600)
	os.WriteFile(filepath.Join(root, "kill-test"), []byte("1"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	if granted := decide(t, bin, repo, home, waitPipeline(t, bin, repo, home), "final", "final-fix", nil); granted["outcome"] != "ok" {
		t.Fatal(granted)
	}
	parked := waitPipeline(t, bin, repo, home)
	if runOf(t, parked)["status"] != "recovery-required" {
		t.Fatalf("final-fix run did not reach the interrupted test stage: %v", runOf(t, parked)["pipeline"])
	}
	os.Remove(filepath.Join(root, "kill-test"))
	recovered := waitProbed(t, bin, repo, home)
	before := recovered["budget"].(map[string]any)
	grant := runOf(t, recovered)["pipeline"].(map[string]any)["final_fix"]
	if before["final_fix_repair"] != float64(1) || before["final_fix_review"] != float64(1) || before["review"] != float64(1) {
		t.Fatalf("predecessor budget: %v", before)
	}
	successor := recoverRequest(t, bin, repo, home, recovered, "new-run", "final-fix-new-run")
	assertBudget := func(view map[string]any, when string) {
		t.Helper()
		after := view["budget"].(map[string]any)
		for _, counter := range []string{"review", "repair", "total", "gate_retries", "final_fix_repair", "final_fix_review"} {
			if after[counter] != before[counter] {
				t.Fatalf("%s reset budget counter %s: before %v after %v", when, counter, before, after)
			}
		}
		a, _ := json.Marshal(grant)
		b, _ := json.Marshal(runOf(t, view)["pipeline"].(map[string]any)["final_fix"])
		if string(a) != string(b) {
			t.Fatalf("%s changed the final-fix grant:\n%s\n%s", when, a, b)
		}
	}
	if successor["outcome"] != "ok" {
		t.Fatal(successor)
	}
	assertBudget(successor, "new-run")
	q := map[string]any{"version": 1, "op": "prepare", "run_id": successor["run_id"], "input_id": successor["input_id"], "gate_id": successor["gate_id"], "expected": successor["token"], "action_id": "re-prepare", "intent": "Change the sample again", "target": runOf(t, successor)["target"]}
	b, _ := json.Marshal(q)
	prepared := axi(t, bin, repo, home, string(b))
	if prepared["outcome"] != "ok" || runOf(t, prepared)["intent"] != "Change the sample again" {
		t.Fatalf("the linked new run could not be prepared again: %v", prepared)
	}
	assertBudget(prepared, "re-Prepare")
	if d := decisionIn(t, func() map[string]any {
		mutate(t, bin, repo, home, prepared, "pipeline", "successor-start", nil)
		return waitPipeline(t, bin, repo, home)
	}()); has(strs(d["options"]), "final-fix") {
		t.Fatalf("the successor was offered a second final-fix grant: %v", d)
	}
}

func stopDaemonAndWait(t *testing.T, home string) {
	t.Helper()
	stopDaemon(t, home)
	waitDaemonExit(t, home)
}

func waitDaemonExit(t *testing.T, home string) {
	t.Helper()
	lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	deadline := time.Now().Add(10 * time.Second)
	for syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		if time.Now().After(deadline) {
			t.Fatal("daemon kept its state lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
}

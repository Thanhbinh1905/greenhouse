package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRerunFailedGateKeepsEarlierStagesAndBudget(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "no documentation", "push": "local fixture", "pr": "local fixture", "ci": "local fixture"}, "repair_limit": 0}
	})
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	v = waitPipeline(t, bin, repo, home)
	if stageOf(t, v, "test")["status"] != "failed" || !has(strs(v["actions"]), "rerun") || !has(strs(v["actions"]), "rerun-from-start") {
		t.Fatalf("a failed stage offers rerun and rerun from start: %v %v", stageOf(t, v, "test"), v["actions"])
	}
	used := runOf(t, v)["review_used"]
	os.Remove(filepath.Join(root, "require-test"))
	v = mutate(t, bin, repo, home, v, "rerun", "rerun-test", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("rerun: %v", v)
	}
	v = waitPipeline(t, bin, repo, home)
	run := runOf(t, v)
	if run["status"] != "locally-ready" || run["review_used"] != used || stageOf(t, v, "review")["runs"].(float64) != 1 || stageOf(t, v, "test")["runs"].(float64) != 2 {
		t.Fatalf("rerun must restart at test only: %v", run["pipeline"])
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 1 {
		t.Fatalf("rerun repeated the review: %v", got)
	}
}

func TestRerunAfterCancelAndRefusalWhileLive(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, localPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "sleep-test"), []byte("30"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	v = waitStatus(t, bin, repo, home, "running")
	for i := 0; i < 200 && stageOf(t, v, "test")["status"] != "running"; i++ {
		v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	}
	if refused := mutate(t, bin, repo, home, v, "rerun", "rerun-live", nil); refused["outcome"] != "refused" {
		t.Fatalf("rerun must refuse while the stage is live: %v", refused)
	}
	v = mutate(t, bin, repo, home, v, "cancel", "abort", nil)
	v = waitPipeline(t, bin, repo, home)
	if runOf(t, v)["status"] != "cancelled" || !has(strs(v["actions"]), "rerun") {
		t.Fatalf("cancel is an abort that leaves the run rerunnable: %v %v", runOf(t, v)["status"], v["actions"])
	}
	os.Remove(filepath.Join(root, "sleep-test"))
	v = mutate(t, bin, repo, home, v, "rerun", "rerun-cancelled", nil)
	v = waitPipeline(t, bin, repo, home)
	if runOf(t, v)["status"] != "needs-decision" && runOf(t, v)["status"] != "locally-ready" {
		t.Fatalf("rerun after abort must continue the pipeline: %v", runOf(t, v)["pipeline"])
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 1 {
		t.Fatalf("rerun after abort repeated the review: %v", got)
	}
}

func TestRerunFromStartCreatesLinkedRunAndKeepsOldOne(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "no documentation", "push": "local fixture", "pr": "local fixture", "ci": "local fixture"}, "repair_limit": 0}
	})
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	old := waitPipeline(t, bin, repo, home)
	oldRun := runOf(t, old)
	v = mutate(t, bin, repo, home, old, "rerun-from-start", "restart", nil)
	if v["outcome"] != "ok" || v["run_id"] == oldRun["id"] {
		t.Fatalf("rerun from start must create a new run: %v", v)
	}
	run := runOf(t, v)
	if run["predecessor"] != oldRun["id"] || run["intent"] != oldRun["intent"] || run["target"] != oldRun["target"] || run["review_used"].(float64) != 0 {
		t.Fatalf("new run must link to the old run with the same intent and target: %v", run)
	}
	q := `{"version":1,"op":"inspect","run_id":"` + oldRun["id"].(string) + `"}`
	previous := runOf(t, axi(t, bin, repo, home, q))
	if previous["successor"] != run["id"] || previous["status"] != "restarted" || previous["review_used"] != oldRun["review_used"] || len(previous["pipeline"].(map[string]any)["stages"].([]any)) == 0 {
		t.Fatalf("old run must stay intact and linked: %v", previous)
	}
	if !strings.Contains(strings.Join(strs(v["actions"]), " "), "pipeline") {
		t.Fatalf("the new run must be ready to run: %v", v["actions"])
	}
}

func stoppedRepairFixture(t *testing.T, stop string) (string, string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home, root, v := onePassWith(t, "codex", "gpt-5.6-luna")
	os.WriteFile(filepath.Join(root, "findings-codex"), []byte("1"), 0600)
	writeExecutable(t, filepath.Join(root, "one-pass-fixer"), "#!/usr/bin/python3\nimport os,sys,time\nroot=os.path.dirname(os.path.abspath(__file__))\nsys.stdin.read()\nmode=open(root+'/fix-mode').read() if os.path.exists(root+'/fix-mode') else ''\nopen('sample.txt','w').write('half '+mode+'\\n' if mode else 'repaired\\n')\nif mode=='untracked': open('notes.txt','w').write('unique-untracked-content-7f3a\\n')\nif mode=='symlink': os.symlink('unique-symlink-target-9e1f','link.txt')\nif mode=='fifo': open('.gitignore','w').write('ignored-pipe\\n'); os.mkfifo('pipe.txt'); os.mkfifo('ignored-pipe')\nif mode=='ignored-directory': open('.gitignore','w').write('ignored-dir/\\n'); os.mkdir('ignored-dir'); os.mkfifo('ignored-dir/pipe'); os.chmod('ignored-dir',0); os.mkdir('blocked-dir'); os.mkdir('blocked-dir/nested'); os.mkdir('blocked-dir/nested/deep'); os.mkfifo('blocked-dir/nested/deep/pipe'); os.chmod('blocked-dir/nested/deep',0); os.chmod('blocked-dir/nested',0); os.chmod('blocked-dir',0); os.mkdir('readonly-dir'); os.mkfifo('readonly-dir/pipe'); os.chmod('readonly-dir',0o500); open('unreadable.txt','w').write('hidden contents\\n'); os.chmod('unreadable.txt',0); os.mkfifo('pipe.txt')\nif mode=='commit': import subprocess; subprocess.run(['git','-c','user.name=F','-c','user.email=f@example.invalid','commit','-qam','fixer commit'],check=True)\nif mode=='cancel': time.sleep(30)\nsys.exit(1 if mode else 0)\n")
	writePrivateJSON(t, filepath.Join(home, "fixer.json"), map[string]any{"executable": filepath.Join(root, "one-pass-fixer"), "model": "fixture/model", "name": "Test", "email": "test@example.invalid", "checks": [][]string{{"/usr/bin/true"}}})
	os.WriteFile(filepath.Join(root, "fix-mode"), []byte(stop), 0600)
	mutate(t, bin, repo, home, v, "review", "review-findings", nil)
	v = waitStatus(t, bin, repo, home, "outstanding-findings")
	q := map[string]any{"version": 1, "op": "repair", "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": "none", "action_id": "repair", "selected": []string{runOf(t, v)["findings"].([]any)[0].(map[string]any)["id"].(string)}}
	b, _ := json.Marshal(q)
	if v = axi(t, bin, repo, home, string(b)); v["outcome"] != "ok" {
		t.Fatalf("repair: %v", v)
	}
	if stop == "cancel" {
		for i := 0; i < 400 && (runOf(t, v)["repair"] == nil || runOf(t, v)["repair"].(map[string]any)["status"] != "fixing"); i++ {
			v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		}
		v = mutate(t, bin, repo, home, v, "cancel", "abort-repair", nil)
		return bin, repo, home, root, waitStatus(t, bin, repo, home, "cancelled")
	}
	v = waitStatus(t, bin, repo, home, "fixer-failed")
	if worktree := runOf(t, v)["repair"].(map[string]any)["worktree"].(string); stop == "commit" && strings.TrimSpace(git(t, worktree, "log", "-1", "--format=%s")) != "fixer commit" {
		t.Fatal("fixture fixer did not commit")
	}
	return bin, repo, home, root, v
}

func TestRerunStoppedSelectedRepairKeepsEvidenceAndCounters(t *testing.T) {
	t.Parallel()
	for _, stop := range []string{"fail", "cancel", "commit"} {
		t.Run(stop, func(t *testing.T) {
			t.Parallel()
			bin, repo, home, root, v := stoppedRepairFixture(t, stop)
			first := runOf(t, v)["repair"].(map[string]any)
			if !has(strs(v["actions"]), "rerun") {
				t.Fatalf("a stopped selected repair must offer rerun: %v", v["actions"])
			}
			os.Remove(filepath.Join(root, "fix-mode"))
			if v = mutate(t, bin, repo, home, v, "rerun", "rerun-repair", nil); v["outcome"] != "ok" {
				t.Fatalf("rerun: %v", v)
			}
			v = waitStatus(t, bin, repo, home, "fixed-not-independently-rereviewed")
			run := runOf(t, v)
			prior := run["prior_repairs"].([]any)
			if run["repair_used"].(float64) != 2 || run["review_used"].(float64) != 1 || len(prior) != 1 || prior[0].(map[string]any)["id"] != first["id"] || prior[0].(map[string]any)["status"] != first["status"] {
				t.Fatalf("rerun must keep the stopped repair and both counters: %v", run)
			}
			kept := prior[0].(map[string]any)
			partial, _ := kept["partial"].(string)
			if !strings.Contains(partial, "+half") || kept["partial_digest"] != bytesDigest([]byte(partial)) || kept["partial_bytes"] != float64(len(partial)) {
				t.Fatalf("the stopped repair's committed and uncommitted edits must be kept with digest and size before the candidate is reset: %v", kept)
			}
			worktree := run["repair"].(map[string]any)["worktree"].(string)
			if worktree != first["worktree"] || strings.TrimSpace(git(t, worktree, "show", "HEAD:sample.txt")) != "repaired" {
				t.Fatalf("rerun must repair the restored candidate: %v", run["repair"])
			}
		})
	}
}

func TestRefusedRepairRerunLeavesCandidateUntouched(t *testing.T) {
	t.Parallel()
	bin, repo, home, _, v := stoppedRepairFixture(t, "fail")
	worktree := runOf(t, v)["repair"].(map[string]any)["worktree"].(string)
	os.Remove(filepath.Join(home, "fixer.json"))
	refused := mutate(t, bin, repo, home, v, "rerun", "rerun-unconfigured", nil)
	v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	run := runOf(t, v)
	b, _ := os.ReadFile(filepath.Join(worktree, "sample.txt"))
	if refused["outcome"] != "refused" || string(b) != "half fail\n" || run["repair_used"].(float64) != 1 || run["prior_repairs"] != nil || run["status"] != "fixer-failed" {
		t.Fatalf("a refused rerun must not clean the candidate or spend budget: %v %q %v", refused["error"], b, run)
	}
}

func priorEvidence(t *testing.T, prior any, content string) string {
	t.Helper()
	kept := prior.(map[string]any)
	partial, _ := kept["partial"].(string)
	if !strings.Contains(partial, content) || kept["partial_digest"] != bytesDigest([]byte(partial)) || kept["partial_bytes"] != float64(len(partial)) {
		t.Fatalf("prior repair must keep %q with its own digest and size: %v", content, kept)
	}
	return kept["partial_digest"].(string)
}

func TestRepeatedRerunsKeepEachFailedRepairsEvidence(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := stoppedRepairFixture(t, "fail")
	os.WriteFile(filepath.Join(root, "fix-mode"), []byte("again"), 0600)
	if v = mutate(t, bin, repo, home, v, "rerun", "rerun-again", nil); v["outcome"] != "ok" {
		t.Fatalf("second attempt: %v", v)
	}
	v = waitStatus(t, bin, repo, home, "fixer-failed")
	os.Remove(filepath.Join(root, "fix-mode"))
	if v = mutate(t, bin, repo, home, v, "rerun", "rerun-third", nil); v["outcome"] != "ok" {
		t.Fatalf("third attempt: %v", v)
	}
	v = waitStatus(t, bin, repo, home, "fixed-not-independently-rereviewed")
	prior := runOf(t, v)["prior_repairs"].([]any)
	if len(prior) != 2 || runOf(t, v)["repair_used"].(float64) != 3 {
		t.Fatalf("two failed attempts must both be kept: %v", runOf(t, v))
	}
	if priorEvidence(t, prior[0], "+half fail") == priorEvidence(t, prior[1], "+half again") {
		t.Fatal("each failed attempt needs its own digest")
	}
}

func TestRerunSkipsUnreadableIgnoredDirectory(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := stoppedRepairFixture(t, "ignored-directory")
	worktree := runOf(t, v)["repair"].(map[string]any)["worktree"].(string)
	ignoredDir := filepath.Join(worktree, "ignored-dir")
	blockedDir := filepath.Join(worktree, "blocked-dir")
	defer func() {
		os.Chmod(ignoredDir, 0700)
		os.RemoveAll(ignoredDir)
		os.Chmod(filepath.Join(worktree, "readonly-dir"), 0700)
		os.RemoveAll(filepath.Join(worktree, "readonly-dir"))
		for _, dir := range []string{filepath.Join(blockedDir, "nested", "deep"), filepath.Join(blockedDir, "nested"), blockedDir} {
			os.Chmod(dir, 0700)
		}
		os.RemoveAll(blockedDir)
	}()
	os.Remove(filepath.Join(root, "fix-mode"))
	if v = mutate(t, bin, repo, home, v, "rerun", "rerun-ignored-dir", nil); v["outcome"] != "ok" {
		t.Fatalf("rerun: %v", v)
	}
	v = waitStatus(t, bin, repo, home, "fixed-not-independently-rereviewed")
	partial := runOf(t, v)["prior_repairs"].([]any)[0].(map[string]any)["partial"].(string)
	if !strings.Contains(partial, "untracked: pipe.txt (fifo)") || strings.Contains(partial, "untracked: ignored-dir") || !strings.Contains(partial, "untracked: blocked-dir (directory unavailable:") || !strings.Contains(partial, "untracked: readonly-dir/pipe (fifo)") || !strings.Contains(partial, "untracked: unreadable.txt (read failed:") {
		t.Fatalf("capture must skip the unreadable ignored directory and retain the outside FIFO: %q", partial)
	}
	worktree = runOf(t, v)["repair"].(map[string]any)["worktree"].(string)
	if _, err := os.Lstat(filepath.Join(worktree, "pipe.txt")); !os.IsNotExist(err) {
		t.Fatalf("rerun cleanup must remove the outside FIFO: %v", err)
	}
	for _, name := range []string{"blocked-dir", "blocked-dir/nested/deep", "readonly-dir", "unreadable.txt"} {
		if _, err := os.Lstat(filepath.Join(worktree, name)); !os.IsNotExist(err) {
			t.Fatalf("rerun cleanup must remove %s: %v", name, err)
		}
	}
	info, err := os.Lstat(ignoredDir)
	if err != nil {
		t.Fatalf("rerun cleanup must preserve the ignored directory: %v", err)
	}
	if info.Mode().Perm() != 0 {
		t.Fatalf("rerun cleanup must not change the ignored directory's permissions: %v", info.Mode().Perm())
	}
}

func TestRerunKeepsUntrackedFIFOEvidence(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := stoppedRepairFixture(t, "fifo")
	os.Remove(filepath.Join(root, "fix-mode"))
	if v = mutate(t, bin, repo, home, v, "rerun", "rerun-fifo", nil); v["outcome"] != "ok" {
		t.Fatalf("rerun: %v", v)
	}
	v = waitStatus(t, bin, repo, home, "fixed-not-independently-rereviewed")
	partial := runOf(t, v)["prior_repairs"].([]any)[0].(map[string]any)["partial"].(string)
	if !strings.Contains(partial, "untracked: pipe.txt (fifo)") || strings.Contains(partial, "untracked: ignored-pipe") {
		t.Fatalf("untracked FIFO must be recorded by name and type without recording ignored entries: %q", partial)
	}
	worktree := runOf(t, v)["repair"].(map[string]any)["worktree"].(string)
	if _, err := os.Lstat(filepath.Join(worktree, "pipe.txt")); !os.IsNotExist(err) {
		t.Fatalf("rerun cleanup must remove the stopped attempt's FIFO: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(worktree, "ignored-pipe")); err != nil {
		t.Fatalf("rerun cleanup must preserve the ignored FIFO: %v", err)
	}
}

func TestRerunKeepsUntrackedSymlinkTarget(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := stoppedRepairFixture(t, "symlink")
	os.Remove(filepath.Join(root, "fix-mode"))
	if v = mutate(t, bin, repo, home, v, "rerun", "rerun-symlink", nil); v["outcome"] != "ok" {
		t.Fatalf("rerun: %v", v)
	}
	v = waitStatus(t, bin, repo, home, "fixed-not-independently-rereviewed")
	priorEvidence(t, runOf(t, v)["prior_repairs"].([]any)[0], "unique-symlink-target-9e1f")
	worktree := runOf(t, v)["repair"].(map[string]any)["worktree"].(string)
	if _, err := os.Lstat(filepath.Join(worktree, "link.txt")); !os.IsNotExist(err) {
		t.Fatal("the restored candidate must not keep the stopped attempt's symlink")
	}
}

func TestRerunKeepsUntrackedFileContents(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := stoppedRepairFixture(t, "untracked")
	os.Remove(filepath.Join(root, "fix-mode"))
	if v = mutate(t, bin, repo, home, v, "rerun", "rerun-untracked", nil); v["outcome"] != "ok" {
		t.Fatalf("rerun: %v", v)
	}
	v = waitStatus(t, bin, repo, home, "fixed-not-independently-rereviewed")
	priorEvidence(t, runOf(t, v)["prior_repairs"].([]any)[0], "unique-untracked-content-7f3a")
	worktree := runOf(t, v)["repair"].(map[string]any)["worktree"].(string)
	if _, err := os.Stat(filepath.Join(worktree, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("the restored candidate must not keep the stopped attempt's new file")
	}
}

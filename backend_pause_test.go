package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setModes(t *testing.T, root, backend string, modes ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, backend+"-modes"), []byte(strings.Join(modes, " ")), 0600); err != nil {
		t.Fatal(err)
	}
}

func pauseOf(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	pause, ok := runOf(t, v)["pause"].(map[string]any)
	if !ok {
		t.Fatalf("no pause: %v", runOf(t, v))
	}
	return pause
}

func TestQuotaPausesOnePassReviewWithoutSpendingBudgetAndResumes(t *testing.T) {
	bin, repo, home, root, v := onePassWith(t, "codex", "gpt-5.6-luna")
	setModes(t, root, "codex", "quota", "success")
	v = mutate(t, bin, repo, home, v, "review", "review-quota", nil)
	v = waitReview(t, bin, repo, home)
	run := runOf(t, v)
	pause := pauseOf(t, v)
	if run["status"] != "paused" || run["review_used"].(float64) != 0 || pause["reason"] != "quota" || pause["backend"] != "codex" || pause["reset_at"] == "" || pause["stage"] != "independent-review" {
		t.Fatalf("quota must park the review without spending its round: status %v used %v pause %v", run["status"], run["review_used"], pause)
	}
	attempts := run["attempts"].([]any)
	if len(attempts) != 1 || attempts[0].(map[string]any)["reason"] != "quota" || attempts[0].(map[string]any)["outcome"] != "paused" {
		t.Fatalf("paused attempt must be recorded: %v", attempts)
	}
	if !strings.Contains(v["explanation"].(string), "Codex quota exhausted") || !has(strs(v["actions"]), "resume") || !has(strs(v["actions"]), "rerun") {
		t.Fatalf("paused envelope needs an explanation and resume/rerun actions: %v %v", v["explanation"], v["actions"])
	}
	firstInput := run["review"].(map[string]any)["input_digest"]
	firstID := run["review"].(map[string]any)["id"]
	paused := v
	v = mutate(t, bin, repo, home, paused, "resume", "resume-review", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("resume: %v", v)
	}
	again := mutate(t, bin, repo, home, paused, "resume", "resume-review", nil)
	if again["outcome"] != "ok" || again["token"] != v["token"] {
		t.Fatalf("replayed resume must return its receipt: %v", again)
	}
	v = waitReview(t, bin, repo, home)
	run = runOf(t, v)
	review := run["review"].(map[string]any)
	if run["status"] != "clean-review" || run["review_used"].(float64) != 1 || review["input_digest"] != firstInput || review["id"] == firstID || run["pause"] != nil {
		t.Fatalf("resume must rerun the same bound review once in a fresh session: %v", run)
	}
	if calls := agentCalls(t, root, "codex"); len(calls) != 2 {
		t.Fatalf("duplicate resume must not start another review: %d calls", len(calls))
	}
}

func pipelineWith(t *testing.T, reviewer, reviewerModel, fixer, fixerModel string) (string, string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := installAgentCLIs(t, home)
	v = mutate(t, bin, repo, home, v, "apply", "backends", map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": reviewer, "reviewer_model": reviewerModel, "fixer_backend": fixer, "fixer_model": fixerModel})
	if v["outcome"] != "ok" {
		t.Fatalf("apply backends: %v", v)
	}
	return bin, repo, home, root, v
}

func TestPipelineReviewPauseKeepsEvidenceAndResumesAtReview(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := pipelineWith(t, "codex", "gpt-5.6-luna", "pi", "fixture/model")
	setModes(t, root, "codex", "quota", "success")
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	v = waitPipeline(t, bin, repo, home)
	run := runOf(t, v)
	p := run["pipeline"].(map[string]any)
	if run["status"] != "paused" || p["status"] != "paused" || stageOf(t, v, "review")["status"] != "paused" || !strings.Contains(stageOf(t, v, "review")["reason"].(string), "paused: backend quota") {
		t.Fatalf("review stage must park as paused: %v", p)
	}
	if stageOf(t, v, "intent")["status"] != "passed" || stageOf(t, v, "sync")["status"] != "passed" || run["review_used"].(float64) != 0 {
		t.Fatalf("completed stages and budget must survive the pause: %v", run)
	}
	syncRuns := stageOf(t, v, "sync")["runs"]
	v = mutate(t, bin, repo, home, v, "resume", "resume-pipeline", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("resume: %v", v)
	}
	v = waitPipeline(t, bin, repo, home)
	run = runOf(t, v)
	if run["status"] != "locally-ready" || run["review_used"].(float64) != 1 || stageOf(t, v, "sync")["runs"] != syncRuns {
		t.Fatalf("resume must continue from review without rerunning earlier stages: %v", run["pipeline"])
	}
}

func TestPipelineRepairPauseRestoresCandidateAndResumes(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := pipelineWith(t, "pi", "fixture/model", "claude", "claude-haiku-5-5")
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	os.WriteFile(filepath.Join(root, "edit-claude"), []byte("repaired\n"), 0600)
	os.WriteFile(filepath.Join(root, "partial-claude"), []byte("half done\n"), 0600)
	setModes(t, root, "claude", "quota", "success")
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	v = waitPipeline(t, bin, repo, home)
	run := runOf(t, v)
	p := run["pipeline"].(map[string]any)
	pause := pauseOf(t, v)
	if run["status"] != "paused" || pause["kind"] != "repair" || pause["backend"] != "claude" || p["repair_used"].(float64) != 0 {
		t.Fatalf("repair must park without spending a gate repair: %v %v", pause, p)
	}
	if pause["auto_resume_at"] != pause["reset_at"] && pause["auto_resume_at"] == "" {
		t.Fatalf("auto mode must schedule a resume after the reported reset: %v", pause)
	}
	worktree := p["worktree"].(string)
	if b, _ := os.ReadFile(filepath.Join(worktree, "sample.txt")); string(b) != "review me\n" {
		t.Fatalf("private candidate must be restored to its committed identity, got %q", b)
	}
	if status := git(t, worktree, "status", "--porcelain"); status != "" {
		t.Fatalf("candidate left dirty: %q", status)
	}
	attempts := run["attempts"].([]any)
	last := attempts[len(attempts)-1].(map[string]any)
	if last["kind"] != "repair" || !strings.Contains(last["partial"].(string), "+half done") {
		t.Fatalf("partial edits must be kept as evidence: %v", last)
	}
	v = mutate(t, bin, repo, home, v, "resume", "resume-repair", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("resume: %v", v)
	}
	v = waitPipeline(t, bin, repo, home)
	run = runOf(t, v)
	p = run["pipeline"].(map[string]any)
	if run["status"] != "locally-ready" || p["repair_used"].(float64) != 1 || len(agentCalls(t, root, "claude")) != 2 {
		t.Fatalf("resumed repair must converge with one gate repair: %v", p)
	}
}

func TestFailedAndPausedAttemptsKeepReportedUsage(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := onePassWith(t, "claude", "claude-haiku-5-5")
	os.WriteFile(filepath.Join(root, "bad-verdict-claude"), []byte("1"), 0600)
	mutate(t, bin, repo, home, v, "review", "review-bad-verdict", nil)
	v = waitStatus(t, bin, repo, home, "inconclusive")
	attempts := runOf(t, v)["attempts"].([]any)
	last := attempts[len(attempts)-1].(map[string]any)
	if last["outcome"] != "inconclusive" || last["cost_usd"].(float64) != 6.0519999999999997e-05 || last["tokens"].(float64) != 55 {
		t.Fatalf("an inconclusive attempt must keep its reported usage: %v", last)
	}
}

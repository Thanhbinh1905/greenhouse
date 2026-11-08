package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRerunFailedGateKeepsEarlierStagesAndBudget(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "no documentation"}, "repair_limit": 0}
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
	bin, repo, home, v := pipelineFixture(t, repairPlan)
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
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "no documentation"}, "repair_limit": 0}
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

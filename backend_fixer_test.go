package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeFixerMustReportExactModelAndTools(t *testing.T) {
	t.Parallel()
	for name, file := range map[string]string{"extra-tool-claude": "Bash", "init-model-claude": "claude-opus-5-5", "extra-model-claude": "claude-opus-5-5"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bin, repo, home, root, v := pipelineWith(t, "pi", "fixture/model", "claude", "claude-haiku-5-5")
			os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
			os.WriteFile(filepath.Join(root, "edit-claude"), []byte("repaired\n"), 0600)
			os.WriteFile(filepath.Join(root, name), []byte(file), 0600)
			submitted := runOf(t, v)["submitted"].(map[string]any)["head"]
			mutate(t, bin, repo, home, v, "pipeline", "start", nil)
			v = waitPipeline(t, bin, repo, home)
			run := runOf(t, v)
			repairs := run["pipeline"].(map[string]any)["repairs"].([]any)
			repair := repairs[len(repairs)-1].(map[string]any)
			if repair["status"] != "fixer-failed" || repair["head"] != "" || run["candidate"] != submitted || !strings.Contains(repair["log"].(string), "claude") {
				t.Fatalf("an invalid claude fixer session must not be committed: %v", repair)
			}
		})
	}
}

func TestConflictFixerMustReportExactModel(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, _, submitted, parked := conflictFixture(t)
	installAgentCLIs(t, home)
	os.WriteFile(filepath.Join(root, "edit-claude"), []byte("resolved\n"), 0600)
	os.WriteFile(filepath.Join(root, "report-model-claude"), []byte("claude-opus-5-5"), 0600)
	v := mutate(t, bin, repo, home, parked, "apply", "claude-fixer", map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "claude", "fixer_model": "claude-haiku-5-5"})
	if v["outcome"] != "ok" {
		t.Fatalf("apply: %v", v)
	}
	if v = mutate(t, bin, repo, home, v, "resolve-conflict", "resolve-mismatch", nil); v["outcome"] != "ok" {
		t.Fatalf("resolve-conflict: %v", v)
	}
	v = waitPipeline(t, bin, repo, home)
	run := runOf(t, v)
	repairs := run["pipeline"].(map[string]any)["repairs"].([]any)
	repair := repairs[len(repairs)-1].(map[string]any)
	worktree := run["pipeline"].(map[string]any)["worktree"].(string)
	if repair["status"] != "fixer-failed" || !strings.Contains(repair["log"].(string), "claude-opus-5-5") || strings.TrimSpace(git(t, worktree, "rev-parse", "HEAD")) != submitted {
		t.Fatalf("a conflict fix from another model must not be committed: %v", repair)
	}
}

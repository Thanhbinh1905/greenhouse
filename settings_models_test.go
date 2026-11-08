package main

import (
	"encoding/json"
	"testing"
)

func TestApplyAndSaveReviewerAndFixerModels(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "openai/gpt-6-luna", "fixer_backend": "pi", "fixer_model": "anthropic/claude-sonnet-4"}
	applied := mutate(t, bin, repo, home, v, "apply", "apply-models", settings)
	if applied["outcome"] != "ok" {
		t.Fatalf("Apply refused reviewer/fixer model settings: %v", applied)
	}
	configs := runOf(t, applied)["configs"].([]any)
	current := configs[len(configs)-1].(map[string]any)["settings"].(map[string]any)
	if current["reviewer_model"] != "openai/gpt-6-luna" || current["fixer_model"] != "anthropic/claude-sonnet-4" {
		t.Fatalf("Apply lost role model selections: %v", current)
	}
	if applied["defaults"].(map[string]any)["reviewer_model"] == "openai/gpt-6-luna" {
		t.Fatal("Apply changed saved defaults")
	}
	v = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	v = mutate(t, bin, repo, home, v, "save", "save-models", settings)
	encoded, _ := json.Marshal(v["defaults"])
	var saved Settings
	if err := json.Unmarshal(encoded, &saved); err != nil || saved.ReviewerModel != "openai/gpt-6-luna" || saved.FixerModel != "anthropic/claude-sonnet-4" {
		t.Fatalf("Save did not persist role models: %s (%v)", encoded, err)
	}
}

func TestReviewerModelChangeStalesHistoricalVerdict(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	mutate(t, bin, repo, home, v, "review", "model-review", nil)
	v = waitReview(t, bin, repo, home)
	old, _ := runOf(t, v)["review"].(map[string]any)
	if old == nil || old["status"] != "outstanding-findings" {
		t.Fatalf("fixture review did not finish: %v", v)
	}
	settings := Settings{Mode: "auto", Preset: "one-pass", Limit: 4, Rules: []string{"unslop"}, ReviewerBackend: "pi", ReviewerModel: "openai/gpt-6-luna", FixerBackend: "pi", FixerModel: "fixture/model"}
	v = mutate(t, bin, repo, home, v, "apply", "change-reviewer-model", settings)
	run := runOf(t, v)
	historical := run["review"].(map[string]any)
	if v["outcome"] != "ok" || historical == nil {
		t.Fatalf("model change Apply refused or discarded review evidence: %v", v)
	}
	if historical["status"] != "stale" || historical["config"].(map[string]any)["model"] != "fixture/model" {
		t.Fatalf("model change did not stale evidence while retaining its model: %v", historical)
	}
}

func TestApplyRejectsInvalidBackendAndModelIdentifiers(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	settings := Settings{Mode: "auto", Preset: "one-pass", Limit: 4, Rules: []string{"unslop"}, ReviewerBackend: "claude", ReviewerModel: "openai/*", FixerBackend: "pi", FixerModel: "openai/gpt-5.6-luna"}
	result := mutate(t, bin, repo, home, v, "apply", "invalid-provider-model", settings)
	if result["outcome"] != "refused" || result["error"] != "invalid settings" {
		t.Fatalf("invalid backend/model was accepted: %v", result)
	}
}

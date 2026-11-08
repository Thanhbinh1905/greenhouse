package main

import (
	"encoding/json"
	"testing"
)

func TestApplyAndSaveReviewerAndFixerModels(t *testing.T) {
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

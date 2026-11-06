package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangedReviewerAndStaleRuleTokenRefuse(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	path := filepath.Join(home, "reviewer.json")
	b, _ := os.ReadFile(path)
	var cfg ReviewerConfig
	json.Unmarshal(b, &cfg)
	cfg.Model = "fixture/other"
	changed, _ := json.Marshal(cfg)
	os.WriteFile(path, changed, 0600)
	refused := mutate(t, bin, repo, home, v, "review", "changed-config", nil)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "trusted reviewer changed") {
		t.Fatal(refused)
	}
	os.WriteFile(path, b, 0600)
	updated := mutate(t, bin, repo, home, v, "resolve-rules", "new-resolution", nil)
	refused = mutate(t, bin, repo, home, v, "review", "old-rule-token", nil)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "stale") {
		t.Fatal(refused)
	}
	if runOf(t, updated)["review_used"] != float64(0) {
		t.Fatal("refusal consumed budget")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(home), "invocations")); !os.IsNotExist(err) {
		t.Fatal("unapproved invocation")
	}
}

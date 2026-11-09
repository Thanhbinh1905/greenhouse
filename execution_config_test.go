package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangedReviewerAndStaleRuleTokenRefuse(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	path := filepath.Join(filepath.Dir(home), "native-bin", "pi")
	saved := path + ".saved"
	if err := os.Rename(path, saved); err != nil {
		t.Fatal(err)
	}
	refused := mutate(t, bin, repo, home, v, "review", "changed-config", nil)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "trusted reviewer changed") {
		t.Fatal(refused)
	}
	os.Rename(saved, path)
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

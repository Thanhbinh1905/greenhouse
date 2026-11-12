package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangedReviewerAndStaleRuleTokenRefuse(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	path := filepath.Join(filepath.Dir(home), "native-bin", "pi")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fallback := filepath.Join(filepath.Dir(home), "fallback-bin")
	if err := os.Mkdir(fallback, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fallback, "pi"), contents, 0700); err != nil {
		t.Fatal(err)
	}
	saved := path + ".saved"
	if err := os.Rename(path, saved); err != nil {
		t.Fatal(err)
	}
	stopDaemon(t, home)
	request, err := json.Marshal(map[string]any{"version": 1, "op": "review", "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": "changed-config", "reason": "test adjustment"})
	if err != nil {
		t.Fatal(err)
	}
	refused := axi(t, bin, repo, home, string(request), "PATH="+fallback+string(os.PathListSeparator)+os.Getenv("PATH"))
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "trusted reviewer changed") {
		t.Fatal(refused)
	}
	if err := os.Rename(saved, path); err != nil {
		t.Fatal(err)
	}
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

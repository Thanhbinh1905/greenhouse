package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuleFailureEligibilityCLI(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	path := filepath.Join(t.TempDir(), "large.json")
	if e := os.WriteFile(path, []byte(strings.Repeat("x", maxPackBytes+1)), 0600); e != nil {
		t.Fatal(e)
	}
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	for i, tc := range []struct {
		rules []string
		ready bool
	}{
		{[]string{"missing"}, false},
		{[]string{"?missing"}, true},
		{[]string{"file:" + path}, false},
		{[]string{"file:relative.json"}, false},
		{[]string{"unslop", "unslop"}, false},
		{[]string{"unslop", "no-comments"}, true},
	} {
		settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": tc.rules}
		key := string(rune('a' + i))
		v = mutate(t, bin, repo, home, v, "apply", "select-"+key, settings)
		v = mutate(t, bin, repo, home, v, "resolve-rules", "resolve-"+key, nil)
		if v["outcome"] != "ok" || v["rules_ready"] != tc.ready {
			t.Fatalf("%v: %v", tc, v)
		}
	}
}
func TestVersionOneRuleExtensionAndDamage(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	h := t.TempDir()
	s, e := load(h)
	if e != nil {
		t.Fatal(e)
	}
	v := handle(s, Request{Version: 1, Op: "attach", Path: repo})
	r := v.Run
	r.ReviewUsed = 2
	r.RepairUsed = 1
	if e = persist(h, s); e != nil {
		t.Fatal(e)
	}
	old, e := load(h)
	if e != nil {
		t.Fatal(e)
	}
	if old.Runs[r.ID].ReviewUsed != 2 || old.Runs[r.ID].RepairUsed != 1 || len(old.Runs[r.ID].RuleSnapshots) != 0 {
		t.Fatal("legacy history reset")
	}
	r.RuleSnapshots = append(r.RuleSnapshots, resolveRules(r))
	if e = persist(h, s); e != nil {
		t.Fatal(e)
	}
	reloaded, e := load(h)
	if e != nil {
		t.Fatal(e)
	}
	if reloaded.Runs[r.ID].ReviewUsed != 2 {
		t.Fatal("usage reset")
	}
	r.RuleSnapshots[0].Rules[0].Bytes = []byte("tampered")
	if e = persist(h, s); e != nil {
		t.Fatal(e)
	}
	if _, e = load(h); e == nil {
		t.Fatal("damaged rule snapshot accepted")
	}
}
func TestRuleSaveDoesNotChangeResolvedRun(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	v = mutate(t, bin, repo, home, v, "resolve-rules", "initial-resolution", nil)
	before, _ := json.Marshal(runOf(t, v)["rule_snapshots"])
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"missing"}}
	saved := mutate(t, bin, repo, home, v, "save", "save-missing", settings)
	after, _ := json.Marshal(runOf(t, saved)["rule_snapshots"])
	if string(before) != string(after) || saved["rules_ready"] != true {
		t.Fatal("Save changed active rules")
	}
	fresh := mutate(t, bin, repo, home, saved, "new", "new-missing", nil)
	if fresh["rules_ready"] != false {
		t.Fatal("New did not use saved selections")
	}
}

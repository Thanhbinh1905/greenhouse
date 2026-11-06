package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLocalRulesImmutableReplayRestart(t *testing.T) {
	bin, repo, home := fixture(t)
	path := filepath.Join(t.TempDir(), "policy.json")
	original := []byte(builtins["unslop"])
	if e := os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"file:" + path, "?missing"}}
	applied := mutate(t, bin, repo, home, v, "apply", "select", settings)
	if applied["rules_ready"] != false {
		t.Fatal("selection marked resolved")
	}
	resolved := mutate(t, bin, repo, home, applied, "resolve-rules", "load", nil)
	if resolved["rules_ready"] != true {
		t.Fatal(resolved)
	}
	if e := os.WriteFile(path, []byte("invalid"), 0600); e != nil {
		t.Fatal(e)
	}
	replay := mutate(t, bin, repo, home, applied, "resolve-rules", "load", nil)
	if !reflect.DeepEqual(replay, resolved) {
		t.Fatal("replay reread file")
	}
	attached := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if !reflect.DeepEqual(runOf(t, attached), runOf(t, resolved)) {
		t.Fatal("attach rewrote snapshot")
	}
	stale := mutate(t, bin, repo, home, applied, "resolve-rules", "stale", nil)
	if stale["outcome"] != "refused" {
		t.Fatal("stale resolution allowed")
	}
	refreshed := mutate(t, bin, repo, home, resolved, "resolve-rules", "refresh", nil)
	if refreshed["rules_ready"] != false {
		t.Fatal("invalid required pack eligible")
	}
	snapshots := runOf(t, refreshed)["rule_snapshots"].([]any)
	if len(snapshots) != 2 {
		t.Fatal(snapshots)
	}
	if !reflect.DeepEqual(snapshots[0], runOf(t, resolved)["rule_snapshots"].([]any)[0]) {
		t.Fatal("history mutated")
	}
	stopDaemon(t, home)
	restarted := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if !reflect.DeepEqual(runOf(t, restarted), runOf(t, refreshed)) {
		t.Fatal("restart lost evidence")
	}
}
func TestPackValidation(t *testing.T) {
	for name, raw := range map[string]string{
		"version":        strings.Replace(builtins["unslop"], `"version":1`, `"version":2`, 1),
		"unknown":        strings.Replace(builtins["unslop"], `"version":1`, `"execute":"sh","version":1`, 1),
		"classification": strings.Replace(builtins["unslop"], "advisory", "correctness", 1),
		"blank":          `{"version":1,"id":"a","criteria":[{"id":"b","classification":"required","text":" ","repair_guidance":"x"}]}`,
		"oversize":       strings.Repeat(" ", maxPackBytes+1),
		"duplicate-key":  `{"version":2,"version":1,"id":"a","criteria":[{"id":"b","classification":"required","text":"x","repair_guidance":"x"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := parsePack([]byte(raw)); e == nil {
				t.Fatal("invalid pack accepted")
			}
		})
	}
	p, _ := parsePack([]byte(builtins["unslop"]))
	p.Criteria = append(p.Criteria, p.Criteria[0])
	raw, _ := json.Marshal(p)
	if _, e := parsePack(raw); e == nil {
		t.Fatal("duplicate criterion accepted")
	}
}

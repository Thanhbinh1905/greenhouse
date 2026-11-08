package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func act(t *testing.T, bin, repo, home string, v map[string]any, op, action string, extra map[string]any) map[string]any {
	t.Helper()
	q := map[string]any{"version": 1, "op": op, "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": action, "reason": "test " + op}
	for key, value := range extra {
		q[key] = value
	}
	b, _ := json.Marshal(q)
	return axi(t, bin, repo, home, string(b))
}

func repositoryOf(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	r, ok := v["repository"].(map[string]any)
	if !ok {
		t.Fatalf("no repository override status: %v", v)
	}
	return r
}

func sourcesOf(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	configs := runOf(t, v)["configs"].([]any)
	sources, ok := configs[len(configs)-1].(map[string]any)["sources"].(map[string]any)
	if !ok {
		t.Fatalf("no per-value sources: %v", configs[len(configs)-1])
	}
	return sources
}

func settingsOf(v map[string]any) map[string]any {
	configs := v["run"].(map[string]any)["configs"].([]any)
	return configs[len(configs)-1].(map[string]any)["settings"].(map[string]any)
}

func TestRepositoryOverrideAppliesOnlyAfterExplicitTrust(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	override := filepath.Join(repo, ".greenhouse.json")
	writeRaw(t, override, `{"version":1,"budget_unit":"rounds","mode":"ask","limit":3}`)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	status := repositoryOf(t, v)
	if status["status"] != "ignored" || !strings.Contains(status["reason"].(string), "not trusted") || status["path"] != override {
		t.Fatalf("untrusted override not shown as ignored: %v", status)
	}
	if s := settingsOf(v); s["mode"] != "auto" || s["limit"] != 4.0 {
		t.Fatalf("untrusted override applied: %v", s)
	}
	if sources := sourcesOf(t, v); sources["mode"] != "built-in default" || sources["limit"] != "built-in default" {
		t.Fatalf("built-in sources missing: %v", sources)
	}
	if got := act(t, bin, repo, home, v, "trust", "trust-wrong", map[string]any{"digest": strings.Repeat("0", 64)}); got["outcome"] != "refused" || !strings.Contains(got["error"].(string), "changed") {
		t.Fatalf("trust accepted a digest the user never saw: %v", got)
	}
	trusted := act(t, bin, repo, home, v, "trust", "trust-1", map[string]any{"digest": status["digest"]})
	if trusted["outcome"] != "ok" || repositoryOf(t, trusted)["status"] != "applied" {
		t.Fatalf("explicit trust refused: %v", trusted)
	}
	if s := settingsOf(trusted); s["mode"] != "auto" {
		t.Fatal("trust silently changed the current run")
	}
	if d := trusted["defaults"].(map[string]any); d["mode"] != "ask" || d["limit"] != 3.0 || trusted["defaults_sources"].(map[string]any)["mode"] != "trusted repository override" {
		t.Fatalf("effective defaults do not show the trusted override: %v %v", d, trusted["defaults_sources"])
	}
	fresh := act(t, bin, repo, home, trusted, "new", "new-1", nil)
	if s := settingsOf(fresh); s["mode"] != "ask" || s["limit"] != 3.0 || s["preset"] != "one-pass" {
		t.Fatalf("trusted override not applied to the new run: %v", s)
	}
	sources := sourcesOf(t, fresh)
	if sources["mode"] != "trusted repository override" || sources["limit"] != "trusted repository override" || sources["preset"] != "built-in default" {
		t.Fatalf("per-value sources wrong: %v", sources)
	}
	applied := act(t, bin, repo, home, fresh, "apply", "apply-1", map[string]any{"settings": map[string]any{"mode": "ask", "preset": "one-pass", "limit": 2, "rules": []string{"unslop", "no-comments"}}})
	if sources := sourcesOf(t, applied); sources["limit"] != "run-local" || sources["mode"] != "trusted repository override" {
		t.Fatalf("run-local source missing: %v", sources)
	}
	saved := act(t, bin, repo, home, applied, "save", "save-user", map[string]any{"settings": map[string]any{"mode": "auto", "preset": "one-pass", "limit": 5, "rules": []string{"unslop", "no-comments"}}})
	if saved["outcome"] != "ok" || saved["defaults_sources"].(map[string]any)["preset"] != "per-user" || saved["defaults_sources"].(map[string]any)["mode"] != "trusted repository override" {
		t.Fatalf("per-user save sources wrong: %v %v", saved["outcome"], saved["defaults_sources"])
	}

	writeRaw(t, override, `{"version":1,"mode":"ask"}`)
	changed := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	if r := repositoryOf(t, changed); r["status"] != "ignored" || !strings.Contains(r["reason"].(string), "changed since it was trusted") {
		t.Fatalf("changed override still applied: %v", r)
	}
	if changed["defaults"].(map[string]any)["limit"] != 5.0 {
		t.Fatal("changed override still narrowed defaults")
	}
}

func TestRepositoryOverrideCannotExpandAuthority(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	override := filepath.Join(repo, ".greenhouse.json")
	for _, tc := range []struct{ name, content, reason string }{
		{"yolo", `{"version":1,"mode":"yolo"}`, "expand"},
		{"limit", `{"version":1,"budget_unit":"rounds","limit":9}`, "above the per-user limit"},
		{"unit", `{"version":1,"budget_unit":"usd","limit":2}`, "budget_unit"},
		{"executable", `{"version":1,"executable":"/bin/sh"}`, "cannot set executable"},
		{"grants", `{"version":1,"grants":[{"authority":"add-files"}]}`, "cannot set executable"},
		{"rule", `{"version":1,"rules":["file:/etc/passwd"]}`, "per-user"},
		{"backend", `{"version":1,"reviewer_backend":"claude","reviewer_model":"claude-sonnet-5-5"}`, "not configured"},
		{"newer", `{"version":2,"mode":"ask"}`, "newer"},
	} {
		writeRaw(t, override, tc.content)
		v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
		v = act(t, bin, repo, home, v, "trust", "trust-"+tc.name, map[string]any{"digest": repositoryOf(t, v)["digest"]})
		r := repositoryOf(t, v)
		if v["outcome"] != "ok" || r["status"] != "ignored" || !strings.Contains(r["reason"].(string), tc.reason) {
			t.Fatalf("%s: expanding override not ignored with reason: %v %v", tc.name, v["error"], r)
		}
		if d := v["defaults"].(map[string]any); d["mode"] != "auto" || d["limit"] != 4.0 {
			t.Fatalf("%s: expanding override changed defaults: %v", tc.name, d)
		}
	}
}

func TestSaveTargetsTrustedRepositoryOverride(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	override := filepath.Join(repo, ".greenhouse.json")
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if r := repositoryOf(t, v); r["status"] != "absent" {
		t.Fatalf("missing override not reported as absent: %v", r)
	}
	narrow := map[string]any{"mode": "ask", "preset": "one-pass", "limit": 2, "rules": []string{"unslop", "no-comments"}}
	if got := act(t, bin, repo, home, v, "save", "save-untrusted", map[string]any{"target": "repository", "settings": narrow}); got["outcome"] != "refused" || !strings.Contains(got["error"].(string), "trust") {
		t.Fatalf("save wrote an untrusted repository override: %v", got)
	}
	if _, err := os.Stat(override); !os.IsNotExist(err) {
		t.Fatal("refused save created the override file")
	}
	v = act(t, bin, repo, home, v, "trust", "trust", map[string]any{"digest": repositoryOf(t, v)["digest"]})
	wide := map[string]any{"mode": "yolo", "preset": "one-pass", "limit": 2, "rules": []string{"unslop", "no-comments"}}
	if got := act(t, bin, repo, home, v, "save", "save-wide", map[string]any{"target": "repository", "settings": wide}); got["outcome"] != "refused" || !strings.Contains(got["error"].(string), "expand") {
		t.Fatalf("save wrote an expanding override: %v", got)
	}
	saved := act(t, bin, repo, home, v, "save", "save-repo", map[string]any{"target": "repository", "settings": narrow})
	if saved["outcome"] != "ok" || repositoryOf(t, saved)["status"] != "applied" {
		t.Fatalf("repository save refused: %v", saved)
	}
	b, _ := os.ReadFile(override)
	var written map[string]any
	json.Unmarshal(b, &written)
	if written["version"] != 1.0 || written["mode"] != "ask" || written["limit"] != 2.0 || written["budget_unit"] != "rounds" || written["preset"] != nil {
		t.Fatalf("override file does not hold only the narrowed values: %s", b)
	}
	if saved["defaults"].(map[string]any)["mode"] != "ask" || saved["defaults_sources"].(map[string]any)["limit"] != "trusted repository override" {
		t.Fatalf("saved override not effective: %v", saved["defaults_sources"])
	}
	if s := settingsOf(saved); s["mode"] != "auto" {
		t.Fatal("save changed the current run")
	}
}

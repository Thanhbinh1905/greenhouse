package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRaw(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func legacyConfigFixture(t *testing.T) (string, string, string, string, map[string]string) {
	t.Helper()
	bin, repo, home := fixture(t)
	root := filepath.Dir(home)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	installPipelineProvider(t, home)
	writeExecutable(t, filepath.Join(root, "fake-pi"), fullReviewer)
	writeExecutable(t, filepath.Join(root, "gate"), gateCommand)
	writePrivateJSON(t, filepath.Join(home, "reviewer.json"), ReviewerConfig{Executable: filepath.Join(root, "fake-pi"), Model: "fixture/model"})
	legacy := map[string]string{
		"reviewer.json": `{"executable":"` + filepath.Join(root, "fake-pi") + `","model":"fixture/model"}`,
		"pipeline.json": `{ "name":"Test","email":"test@example.invalid","test":[["` + filepath.Join(root, "gate") + `","test"]],"repair_limit":2,"not_applicable":{"lint":"none","docs":"none","push":"local","pr":"local","ci":"local"} }`,
	}
	for name, content := range legacy {
		writeRaw(t, filepath.Join(home, name), content)
	}
	providers, err := os.ReadFile(filepath.Join(home, "providers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var unversioned map[string]any
	json.Unmarshal(providers, &unversioned)
	delete(unversioned, "version")
	b, _ := json.Marshal(unversioned)
	legacy["providers.json"] = string(b)
	writeRaw(t, filepath.Join(home, "providers.json"), string(b))
	return bin, repo, home, target, legacy
}

func prepareRequest(v map[string]any, action, target string) string {
	b, _ := json.Marshal(map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": v["gate_id"], "action_id": action, "intent": "Review sample change", "target": target})
	return string(b)
}

func configEntry(t *testing.T, v map[string]any, file string) map[string]any {
	t.Helper()
	for _, item := range v["config"].([]any) {
		if entry := item.(map[string]any); entry["file"] == file {
			return entry
		}
	}
	t.Fatalf("no config status for %s: %v", file, v["config"])
	return nil
}

func TestLegacyConfigurationMigratesExplicitlyWithBackup(t *testing.T) {
	t.Parallel()
	bin, repo, home, target, legacy := legacyConfigFixture(t)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if entry := configEntry(t, v, "pipeline.json"); entry["status"] != "older" || entry["version"] != 1.0 || entry["current"] != 2.0 {
		t.Fatalf("legacy pipeline.json not reported as older: %v", entry)
	}
	if !has(strs(v["actions"]), "migrate") || !strings.Contains(strings.Join(strs(v["blocked"]), "\n"), "pipeline.json uses schema version 1") {
		t.Fatalf("migration not offered: %v %v", v["actions"], v["blocked"])
	}
	v = mutate(t, bin, repo, home, v, "apply", "full", map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
	refused := axi(t, bin, repo, home, prepareRequest(v, "prepare-legacy", target))
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "schema version 1") || !strings.Contains(refused["error"].(string), "migrate") {
		t.Fatalf("legacy configuration was used without migration: %v", refused)
	}
	for name, content := range legacy {
		if b, _ := os.ReadFile(filepath.Join(home, name)); string(b) != content {
			t.Fatalf("%s changed before explicit migration", name)
		}
	}
	request := `{"version":1,"op":"migrate","action_id":"migrate-1","reason":"upgrade configuration"}`
	migrated := axi(t, bin, repo, home, request)
	if migrated["outcome"] != "ok" {
		t.Fatalf("migration refused: %v", migrated)
	}
	records := migrated["migrations"].([]any)
	if len(records) != 1 {
		t.Fatalf("want one record per legacy file: %v", records)
	}
	for _, item := range records {
		record := item.(map[string]any)
		name := record["file"].(string)
		if record["from"] != 1.0 || record["to"] != 2.0 || record["actor"] != "axi" || record["reason"] != "upgrade configuration" || record["action_id"] != "migrate-1" {
			t.Fatalf("incomplete migration record: %v", record)
		}
		backup, err := os.ReadFile(filepath.Join(home, record["backup"].(string)))
		if err != nil || string(backup) != legacy[name] || record["before"] != bytesDigest(backup) {
			t.Fatalf("backup of %s does not hold the original bytes: %v %q", name, err, backup)
		}
		info, _ := os.Stat(filepath.Join(home, record["backup"].(string)))
		after, _ := os.ReadFile(filepath.Join(home, name))
		if info.Mode().Perm() != 0600 || record["after"] != bytesDigest(after) {
			t.Fatalf("backup mode or after digest wrong: %v %v", info.Mode(), record)
		}
		var old, current map[string]json.RawMessage
		json.Unmarshal([]byte(legacy[name]), &old)
		json.Unmarshal(after, &current)
		if string(current["version"]) != "2" {
			t.Fatalf("%s lacks the current version: %s", name, after)
		}
		for key, value := range old {
			if string(current[key]) != string(value) {
				t.Fatalf("%s lost %s in migration: %s", name, key, after)
			}
		}
		if name == "pipeline.json" && string(current["budget_unit"]) != `"rounds"` {
			t.Fatalf("pipeline.json budget unit not made explicit: %s", after)
		}
	}
	if replay := axi(t, bin, repo, home, request); replay["outcome"] != "ok" || len(replay["migrations"].([]any)) != 1 {
		t.Fatalf("replay changed the recorded migration: %v", replay)
	}
	v = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if configEntry(t, v, "pipeline.json")["status"] != "current" || has(strs(v["actions"]), "migrate") {
		t.Fatalf("migrated configuration still reported as older: %v", v["config"])
	}
	if prepared := axi(t, bin, repo, home, prepareRequest(v, "prepare-current", target)); prepared["outcome"] != "ok" {
		t.Fatalf("migrated configuration refused: %v", prepared)
	}
}

func TestNewerConfigurationVersionRefusesClearly(t *testing.T) {
	t.Parallel()
	bin, repo, home, target, _ := legacyConfigFixture(t)
	root := filepath.Dir(home)
	newer := `{"version":3,"budget_unit":"rounds","name":"` + filepath.Base(root) + `"}`
	writeRaw(t, filepath.Join(home, "pipeline.json"), newer)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if entry := configEntry(t, v, "pipeline.json"); entry["status"] != "newer" || !strings.Contains(entry["error"].(string), "newer than this Greenhouse supports (2)") {
		t.Fatalf("newer version not reported: %v", entry)
	}
	refused := axi(t, bin, repo, home, `{"version":1,"op":"migrate","action_id":"migrate-newer","reason":"try"}`)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "pipeline.json: schema version 3 is newer") {
		t.Fatalf("migration accepted a newer file: %v", refused)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "pipeline.json")); string(b) != newer {
		t.Fatal("refused migration changed the newer file")
	}
	if b, _ := os.ReadFile(filepath.Join(home, "providers.json")); strings.Contains(string(b), `"version"`) {
		t.Fatal("a refused migration still migrated another file")
	}
	v = mutate(t, bin, repo, home, v, "apply", "full-newer", map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
	refused = axi(t, bin, repo, home, prepareRequest(v, "prepare-newer", target))
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "newer than this Greenhouse supports") {
		t.Fatalf("newer configuration was read: %v", refused)
	}
}

func TestBudgetUnitIsNeverReinterpreted(t *testing.T) {
	t.Parallel()
	bin, repo, home, target, _ := legacyConfigFixture(t)
	axi(t, bin, repo, home, `{"version":1,"op":"migrate","action_id":"migrate","reason":"upgrade"}`)
	plan, _ := os.ReadFile(filepath.Join(home, "pipeline.json"))
	writeRaw(t, filepath.Join(home, "pipeline.json"), strings.Replace(string(plan), `"budget_unit":"rounds"`, `"budget_unit":"usd"`, 1))
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	v = mutate(t, bin, repo, home, v, "apply", "full", map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
	refused := axi(t, bin, repo, home, prepareRequest(v, "prepare-usd", target))
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), `budget_unit "usd"`) {
		t.Fatalf("a different budget unit was accepted: %v", refused)
	}
	writeRaw(t, filepath.Join(home, "pipeline.json"), strings.Replace(string(plan), `"budget_unit":"rounds",`, ``, 1))
	refused = axi(t, bin, repo, home, prepareRequest(v, "prepare-missing-unit", target))
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "budget_unit") {
		t.Fatalf("a current plan without a budget unit was accepted: %v", refused)
	}
}

func TestNewerRulePackVersionRefusesClearly(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	pack := filepath.Join(filepath.Dir(home), "pack.json")
	writeRaw(t, pack, strings.Replace(builtins["unslop"], `"version":1`, `"version":2`, 1))
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	v = mutate(t, bin, repo, home, v, "apply", "pack", map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"file:" + pack}})
	v = mutate(t, bin, repo, home, v, "resolve-rules", "resolve", nil)
	rules := v["rules"].([]any)
	if got := rules[0].(map[string]any)["error"]; got == nil || !strings.Contains(got.(string), "rule pack schema version 2 is newer than this Greenhouse supports (1)") {
		t.Fatalf("newer rule pack error unclear: %v", rules)
	}
}

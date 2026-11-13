package engine

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDamagedStoreRefusedBeforeBinding(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	stopDaemon(t, home)
	b, _ := os.ReadFile(filepath.Join(home, "state.json"))
	var s Store
	json.Unmarshal(b, &s)
	s.Runs[v["run_id"].(string)].Configs = nil
	persist(home, &s)
	before, _ := os.ReadFile(filepath.Join(home, "state.json"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "daemon")
	cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+home)
	if b, e := cmd.CombinedOutput(); e == nil || ctx.Err() != nil {
		t.Fatalf("damaged store accepted: %s", b)
	}
	after, _ := os.ReadFile(filepath.Join(home, "state.json"))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("damaged evidence rewritten")
	}
}

type legacySettings struct {
	Mode   string   `json:"mode"`
	Preset string   `json:"preset"`
	Limit  int      `json:"limit"`
	Rules  []string `json:"rules"`
}

type legacySnapshot struct {
	ID       string         `json:"id"`
	Settings legacySettings `json:"settings"`
	Source   string         `json:"source"`
}

func TestLegacyCompletedReviewAndResolvedRulesSurviveUpgrade(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, "")
	mutate(t, bin, repo, home, v, "review", "legacy-review", nil)
	v = waitReview(t, bin, repo, home)
	original := runOf(t, v)
	originalReview := original["review"].(map[string]any)
	if originalReview["status"] != "outstanding-findings" || v["rules_ready"] != true || len(original["rule_snapshots"].([]any)) == 0 {
		t.Fatal("fixture lacks completed review and resolved rules")
	}
	stopDaemon(t, home)
	state, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var store Store
	if err := json.Unmarshal(state, &store); err != nil {
		t.Fatal(err)
	}
	run := store.Runs[v["run_id"].(string)]
	run.Review.FindingIdentity = ""
	for i := range run.Review.Result.Findings {
		f := &run.Review.Result.Findings[i]
		f.ID = Digest([]any{f.Origin, f.File, f.Start, f.End, f.Rule, strings.Join(strings.Fields(f.Explanation), " ")})
	}
	legacyFindings := append([]Finding(nil), run.Review.Result.Findings...)
	legacyDigests := map[string]string{}
	for i := range run.Configs {
		cfg := run.Configs[i]
		legacy := legacySnapshot{ID: cfg.ID, Settings: legacySettings{Mode: cfg.Settings.Mode, Preset: cfg.Settings.Preset, Limit: cfg.Settings.Limit, Rules: cfg.Settings.Rules}, Source: cfg.Source}
		legacyDigests[cfg.ID] = Digest(legacy)
	}
	for i := range run.RuleSnapshots {
		run.RuleSnapshots[i].ConfigDigest = legacyDigests[run.RuleSnapshots[i].ConfigID]
	}
	legacyConfig := struct {
		Executable string `json:"executable"`
		Model      string `json:"model"`
	}{run.Review.Config.Executable, run.Review.Config.Model}
	run.Review.ConfigDigest = Digest(legacyConfig)
	run.Review.Config.Provider = ProviderConfig{}
	run.Review.Config.Native = false
	run.Review.Config.DisabledMCP = ""
	run.Review.Config.Backend = ""
	run.Review.Args = legacyAgentArgs(run.Review.Config, "reviewer", verdictSchema(run.Review.Full, len(run.Review.Responses) > 0), reviewerPrompt)
	if run.Reviewer != nil {
		run.Reviewer.Provider = ProviderConfig{}
		run.Reviewer.Native = false
		run.Reviewer.Backend = ""
	}
	for i := range run.Configs {
		run.Configs[i].Settings.ReviewerBackend = ""
		run.Configs[i].Settings.ReviewerModel = ""
		run.Configs[i].Settings.FixerBackend = ""
		run.Configs[i].Settings.FixerModel = ""
	}
	store.Defaults.ReviewerBackend, store.Defaults.ReviewerModel = "", ""
	store.Defaults.FixerBackend, store.Defaults.FixerModel = "", ""
	persist(home, &store)
	upgradedStore, err := load(home)
	if err != nil {
		t.Fatalf("legacy evidence rejected by upgrade: %v", err)
	}
	if !reflect.DeepEqual(upgradedStore.Runs[run.ID].Review.Result.Findings, legacyFindings) {
		t.Fatal("stored legacy finding IDs changed during upgrade")
	}
	persist(home, upgradedStore)
	upgraded := axi(t, bin, repo, home, `{"version":1,"op":"inspect","run_id":"`+v["run_id"].(string)+`"}`)
	loaded := runOf(t, upgraded)
	loadedSnapshots := loaded["rule_snapshots"].([]any)
	originalSnapshots := original["rule_snapshots"].([]any)
	if len(loadedSnapshots) != len(originalSnapshots) {
		t.Fatal("resolved-rule snapshots were lost during upgrade")
	}
	for i := range loadedSnapshots {
		loadedSnapshot := loadedSnapshots[i].(map[string]any)
		originalSnapshot := originalSnapshots[i].(map[string]any)
		delete(loadedSnapshot, "config_digest")
		delete(originalSnapshot, "config_digest")
		if !reflect.DeepEqual(loadedSnapshot, originalSnapshot) {
			t.Fatal("resolved-rule evidence changed during upgrade")
		}
	}
	if loaded["status"] != original["status"] {
		t.Fatal("historical run status changed during upgrade")
	}
	if loaded["review"].(map[string]any)["input_digest"] != originalReview["input_digest"] || loaded["review"].(map[string]any)["status"] != originalReview["status"] {
		t.Fatal("completed review evidence changed during upgrade")
	}
	settings := Settings{Mode: "auto", Preset: "one-pass", Limit: 4, Rules: []string{"unslop", "no-comments"}, ReviewerBackend: "pi", ReviewerModel: "openai/changed-model", FixerBackend: "pi", FixerModel: "openai/gpt-5.6-luna"}
	changed := mutate(t, bin, repo, home, upgraded, "apply", "change-model", settings)
	changedReview := runOf(t, changed)["review"].(map[string]any)
	if changedReview["status"] != "stale" || changedReview["input_digest"] != originalReview["input_digest"] || len(changedReview["result"].(map[string]any)["findings"].([]any)) == 0 {
		t.Fatal("configuration change discarded historical review evidence")
	}
}

func TestRetainedUsageAndLimitEdits(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	stopDaemon(t, home)
	b, _ := os.ReadFile(filepath.Join(home, "state.json"))
	var s Store
	json.Unmarshal(b, &s)
	r := s.Runs[v["run_id"].(string)]
	r.ReviewUsed = 2
	r.RepairUsed = 1
	persist(home, &s)
	v = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	settings := Settings{Mode: "auto", Preset: "one-pass", Limit: 2, Rules: []string{"unslop"}, ReviewerBackend: "pi", ReviewerModel: "openai/gpt-5.6-luna", FixerBackend: "pi", FixerModel: "openai/gpt-5.6-luna"}
	if mutate(t, bin, repo, home, v, "apply", "reduce", settings)["outcome"] != "refused" {
		t.Fatal("consumed usage erased")
	}
	settings.Limit = 8
	applied := mutate(t, bin, repo, home, v, "apply", "increase", settings)
	if applied["outcome"] != "ok" {
		t.Fatal(applied)
	}
	rr := runOf(t, applied)
	if rr["review_used"] != float64(2) || rr["repair_used"] != float64(1) {
		t.Fatal("usage reset")
	}
	events := rr["events"].([]any)
	last := events[len(events)-1].(map[string]any)
	if last["used"] != float64(3) || last["before"] != float64(4) || last["after"] != float64(8) {
		t.Fatal("missing cap provenance")
	}
}

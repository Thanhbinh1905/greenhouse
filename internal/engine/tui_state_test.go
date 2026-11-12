package engine

import (
	"slices"
	"testing"
)

func tuiRun(status string) *Run {
	return &Run{ID: "run", Input: "input", Token: "token", Status: status, Stage: "intent", Candidate: "c", Configs: []Snapshot{{Settings: Settings{Mode: "auto", Preset: "one-pass", Limit: 4, ReviewerBackend: "claude", FixerBackend: "claude"}}}}
}

func TestPrimaryActionFollowsControllerState(t *testing.T) {
	prepared := func(status string) *Run {
		r := tuiRun(status)
		r.Intent, r.Target, r.Reviewer = "fix it", "abc", &ReviewerConfig{Backend: "claude"}
		return r
	}
	findings := prepared("outstanding-findings")
	findings.Findings = []Finding{{ID: "f1", Priority: "P1", Action: "fix"}}
	paused := prepared("paused")
	paused.Pause = &BackendPause{Reason: "quota"}
	exhausted := prepared("unconfigured")
	exhausted.ReviewUsed = 4
	recovering := prepared("recovery-required")
	recovering.Recovery = &Recovery{ID: "gate", Options: []string{"inspect"}}
	cases := []struct {
		name    string
		v       Response
		agents  []string
		want    string
		running bool
	}{
		{"no agent installed", Response{Run: tuiRun("unconfigured"), Actions: []string{"attach", "inspect", "apply", "save", "resolve-rules", "prepare", "new"}}, nil, "", false},
		{"first run", Response{Run: tuiRun("unconfigured"), Actions: []string{"attach", "inspect", "apply", "save", "resolve-rules", "prepare", "new"}}, []string{"claude"}, "run", false},
		{"rules next", Response{Run: prepared("unconfigured"), Actions: []string{"attach", "inspect", "apply", "save", "resolve-rules", "prepare", "new"}}, []string{"claude"}, "run", false},
		{"review only when allowed", Response{Run: prepared("unconfigured"), RulesReady: true, Actions: []string{"attach", "inspect", "apply", "save", "resolve-rules", "prepare", "review", "new"}}, []string{"claude"}, "run", false},
		{"budget exhausted", Response{Run: exhausted, RulesReady: true, Actions: []string{"attach", "inspect", "apply", "save", "resolve-rules", "prepare", "new"}}, []string{"claude"}, "", false},
		{"rules loaded after review", Response{Run: findings, Actions: []string{"attach", "inspect", "apply", "save", "resolve-rules", "new"}}, []string{"claude"}, "resolve-rules", false},
		{"running has no primary", Response{Run: prepared("running"), RulesReady: true, Actions: []string{"attach", "inspect", "cancel"}}, []string{"claude"}, "", true},
		{"findings", Response{Run: findings, RulesReady: true, Actions: []string{"attach", "inspect", "apply", "save", "resolve-rules", "repair", "new", "rerun-from-start"}}, []string{"claude"}, "findings", false},
		{"paused resumes", Response{Run: paused, RulesReady: true, Actions: []string{"attach", "inspect", "apply", "save", "resolve-rules", "resume", "rerun", "new"}}, []string{"claude"}, "rerun", false},
		{"decision first", Response{Run: prepared("needs-decision"), Decision: &Decision{ID: "d", Kind: "cap", Options: []string{"accept-head"}}, RulesReady: true, Actions: []string{"attach", "inspect", "apply", "decide", "new"}}, []string{"claude"}, "decide", false},
		{"recovery first", Response{Run: recovering, Actions: []string{"attach", "inspect", "new", "recover"}}, []string{"claude"}, "recover", false},
		{"host owned follows only", Response{Run: prepared("unconfigured"), RulesReady: true, Actions: []string{"attach", "inspect"}}, []string{"claude"}, "", false},
		{"source moved offers new", Response{Run: prepared("stale"), Actions: []string{"attach", "inspect", "new"}}, []string{"claude"}, "new", false},
	}
	for _, c := range cases {
		got := primaryAction(c.v, c.agents)
		if got.ID != c.want {
			t.Errorf("%s: primary %q, want %q", c.name, got.ID, c.want)
		}
		if got.ID != "" && got.Label == "" {
			t.Errorf("%s: primary action has no label", c.name)
		}
		if got.Op != "" && !slices.Contains(c.v.Actions, got.Op) {
			t.Errorf("%s: primary op %q is not allowed by %v", c.name, got.Op, c.v.Actions)
		}
		if busy(c.v.Run) != c.running {
			t.Errorf("%s: busy %t, want %t", c.name, busy(c.v.Run), c.running)
		}
	}
}

func TestMenuOffersOnlyAllowedMutations(t *testing.T) {
	all := []string{"resolve-rules", "prepare", "review", "pipeline", "decide", "resolve-conflict", "repair", "handback", "cancel", "rerun", "resume", "cancel-auto-resume", "rerun-from-start", "new", "recover", "migrate", "apply", "save"}
	r := tuiRun("unconfigured")
	r.Recovery = &Recovery{ID: "g", Options: []string{"inspect"}}
	r.Pause = &BackendPause{Reason: "quota", AutoResumeAt: "2026-11-10T10:00:00Z"}
	for _, allowed := range [][]string{{"attach", "inspect"}, {"attach", "inspect", "apply"}, append([]string{"attach", "inspect"}, all...)} {
		v := Response{Run: r, Actions: allowed, Decision: &Decision{ID: "d", Options: []string{"accept-head"}}}
		for _, item := range menuItems(v, true) {
			if item.Op != "" && !slices.Contains(allowed, item.Op) {
				t.Errorf("menu offers %q (%s) while actions are %v", item.Op, item.Label, allowed)
			}
		}
		for _, item := range menuItems(v, false) {
			if item.Op != "" {
				t.Errorf("menu offers mutation %q while state is not fresh", item.Op)
			}
		}
	}
	generic := Response{Run: r, Actions: []string{"attach", "inspect", "apply"}}
	for _, item := range menuItems(generic, true) {
		if item.Op != "" && item.Op != "apply" {
			t.Errorf("generic apply permission enabled %q", item.Op)
		}
	}
}

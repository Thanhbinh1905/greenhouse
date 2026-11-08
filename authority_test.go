package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func authorityFixture(t *testing.T, mode string, limit int, plan func(root string) map[string]any) (string, string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home, v := pipelineFixture(t, plan)
	v = mutate(t, bin, repo, home, v, "apply", "mode-"+mode, map[string]any{"mode": mode, "preset": "full", "limit": limit, "rules": []string{"unslop"}})
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	return bin, repo, home, filepath.Dir(home), v
}

func decide(t *testing.T, bin, repo, home string, v map[string]any, action, choice string, extra map[string]any) map[string]any {
	t.Helper()
	q := map[string]any{"version": 1, "op": "decide", "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": action, "choice": choice, "reason": "operator reviewed the " + choice + " option"}
	for k, val := range extra {
		q[k] = val
	}
	b, _ := json.Marshal(q)
	return axi(t, bin, repo, home, string(b))
}

func decisionIn(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	d, ok := v["decision"].(map[string]any)
	if !ok {
		t.Fatalf("no open decision: %v", runOf(t, v)["pipeline"])
	}
	return d
}

func strs(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, x := range list {
		out = append(out, x.(string))
	}
	return out
}

func budgetIn(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	b, ok := v["budget"].(map[string]any)
	if !ok {
		t.Fatalf("no budget: %v", v)
	}
	return b
}

func TestAutoRepairsFindingsWithinCombinedCap(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := authorityFixture(t, "auto", 6, repairPlan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("clean"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	p := run["pipeline"].(map[string]any)
	if run["status"] != "locally-ready" || p["outcome"] != "repaired-pass" {
		t.Fatalf("auto did not repair findings to a repaired pass: %v", p)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["review"] != 1 {
		t.Fatalf("finding repair invocations %v", got)
	}
	b := budgetIn(t, done)
	if b["review"] != float64(2) || b["repair"] != float64(1) || b["total"] != float64(3) || b["limit"] != float64(6) || b["gate_retries"] != float64(0) {
		t.Fatalf("combined budget wrong: %v", b)
	}
	repair := p["repairs"].([]any)[0].(map[string]any)
	if repair["stage"] != "review" || repair["actor"] != "auto" || len(repair["selected"].([]any)) != 1 {
		t.Fatalf("finding repair record: %v", repair)
	}
	resolutions := p["resolutions"].([]any)
	if len(resolutions) != 1 || resolutions[0].(map[string]any)["repair"] != repair["id"] || resolutions[0].(map[string]any)["candidate"] != run["candidate"] {
		t.Fatalf("finding not resolved by repair plus clean rereview: %v", resolutions)
	}
	if done["decision"] != nil {
		t.Fatalf("auto opened a decision: %v", done["decision"])
	}
}

func TestAskGroupsActionableFindingsIntoOneDecision(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := authorityFixture(t, "ask", 6, repairPlan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("mixed"), 0600)
	os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("clean"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	d := decisionIn(t, parked)
	findings := runOf(t, parked)["findings"].([]any)
	if d["kind"] != "findings" || len(strs(d["findings"])) != 2 || len(findings) != 3 || parked["gate_id"] != d["id"] {
		t.Fatalf("ask did not group the two actionable findings into one request: %v %v", d, findings)
	}
	for _, f := range findings {
		if f.(map[string]any)["priority"] == "P3" && has(strs(d["findings"]), f.(map[string]any)["id"].(string)) {
			t.Fatal("informational finding created a question")
		}
	}
	if !has(strs(parked["actions"]), "decide") || has(strs(parked["actions"]), "pipeline") {
		t.Fatalf("actions %v", parked["actions"])
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); len(got) != 0 {
		t.Fatalf("ask repaired before authorization: %v", got)
	}
	if refused := mutate(t, bin, repo, home, parked, "pipeline", "continue-past-decision", nil); refused["outcome"] != "refused" {
		t.Fatal("pipeline continued past an open decision")
	}
	if refused := decide(t, bin, repo, home, parked, "bad-choice", "accept-head", nil); refused["outcome"] != "refused" {
		t.Fatal("decision accepted a choice it does not offer")
	}
	answered := decide(t, bin, repo, home, parked, "authorize", "repair", nil)
	if answered["outcome"] != "ok" {
		t.Fatal(answered)
	}
	if replay := decide(t, bin, repo, home, parked, "authorize", "repair", nil); replay["token"] != answered["token"] || replay["outcome"] != "ok" {
		t.Fatalf("duplicate action did not return its receipt: %v", replay)
	}
	if stale := decide(t, bin, repo, home, parked, "authorize-again", "repair", nil); stale["outcome"] != "refused" {
		t.Fatal("stale decision gate accepted a new action")
	}
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	p := run["pipeline"].(map[string]any)
	if run["status"] != "locally-ready" || p["outcome"] != "repaired-pass" {
		t.Fatalf("authorized repair did not converge: %v", p)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["review"] != 1 {
		t.Fatalf("one grouped authorization must start one repair: %v", got)
	}
	input, _ := os.ReadFile(filepath.Join(root, "fix-input.json"))
	var fix struct{ Selected []Finding }
	json.Unmarshal(input, &fix)
	if len(fix.Selected) != 2 {
		t.Fatalf("fixer did not receive the grouped findings: %s", input)
	}
	answer := p["decisions"].([]any)[0].(map[string]any)["answer"].(map[string]any)
	if answer["actor"] != "axi" || answer["reason"] != "operator reviewed the repair option" || answer["action"] != "authorize" {
		t.Fatalf("decision provenance: %v", answer)
	}
	if review := stageOf(t, done, "review"); !strings.Contains(review["reason"].(string), "1 informational") {
		t.Fatalf("informational finding not visible: %v", review)
	}
}

func capFixture(t *testing.T, mode string, limit int, plan func(root string) map[string]any) (string, string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home, root, v := authorityFixture(t, mode, limit, plan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	os.WriteFile(filepath.Join(root, "fix-content"), []byte("APPEND"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	return bin, repo, home, root, waitPipeline(t, bin, repo, home)
}

func TestCapExhaustionAndIncreaseKeepUsage(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, parked := capFixture(t, "auto", 3, repairPlan)
	d := decisionIn(t, parked)
	if d["kind"] != "cap" || d["user_only"] != false || len(strs(d["findings"])) != 1 {
		t.Fatalf("cap decision: %v", d)
	}
	for _, option := range []string{"accept-head", "final-fix", "increase-cap", "rethink", "discard"} {
		if !has(strs(d["options"]), option) {
			t.Fatalf("cap decision lacks %s: %v", option, d["options"])
		}
	}
	if b := budgetIn(t, parked); b["review"] != float64(2) || b["repair"] != float64(1) || b["total"] != float64(3) {
		t.Fatalf("usage at cap: %v", b)
	}
	if runOf(t, parked)["status"] != "needs-decision" || stageOf(t, parked, "review")["status"] != "needs-decision" {
		t.Fatal(runOf(t, parked)["pipeline"])
	}
	if refused := decide(t, bin, repo, home, parked, "lower", "increase-cap", map[string]any{"limit": 2}); refused["outcome"] != "refused" {
		t.Fatal("increase-cap accepted a lower limit")
	}
	raised := decide(t, bin, repo, home, parked, "raise", "increase-cap", map[string]any{"limit": 5})
	if raised["outcome"] != "ok" {
		t.Fatal(raised)
	}
	var event map[string]any
	for _, e := range runOf(t, raised)["events"].([]any) {
		if e.(map[string]any)["kind"] == "cap-increase" {
			event = e.(map[string]any)
		}
	}
	if event == nil || event["before"] != float64(3) || event["after"] != float64(5) || event["used"] != float64(3) || event["review"] != float64(2) || event["repair"] != float64(1) || event["actor"] != "axi" || event["reason"] != "operator reviewed the increase-cap option" {
		t.Fatalf("cap increase provenance: %v", event)
	}
	again := waitPipeline(t, bin, repo, home)
	if b := budgetIn(t, again); b["review"] != float64(3) || b["repair"] != float64(2) || b["total"] != float64(5) || b["limit"] != float64(5) {
		t.Fatalf("increase reset or lost usage: %v", b)
	}
	if decisionIn(t, again)["kind"] != "cap" {
		t.Fatal("second exhaustion did not park")
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 3 {
		t.Fatalf("cap increase reran review on an unchanged candidate: %v", got)
	}
	if refused := mutate(t, bin, repo, home, again, "apply", "reduce-below", map[string]any{"mode": "auto", "preset": "full", "limit": 4, "rules": []string{"unslop"}}); refused["outcome"] != "refused" {
		t.Fatal("reduction below consumed usage accepted")
	}
	raisedAgain := mutate(t, bin, repo, home, again, "apply", "raise-8", map[string]any{"mode": "ask", "preset": "full", "limit": 8, "rules": []string{"unslop"}})
	reduced := mutate(t, bin, repo, home, raisedAgain, "apply", "reduce-6", map[string]any{"mode": "ask", "preset": "full", "limit": 6, "rules": []string{"unslop"}})
	if b := budgetIn(t, reduced); reduced["outcome"] != "ok" || b["total"] != float64(5) || b["limit"] != float64(6) {
		t.Fatalf("reduction erased usage: %v %v", reduced["error"], b)
	}
}

func strictPlan(root string) map[string]any {
	return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "no documentation"}}
}

func TestAcceptHeadBindsExactCandidateAndNeverPassesGates(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := authorityFixture(t, "auto", 1, strictPlan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	if decisionIn(t, parked)["kind"] != "cap" {
		t.Fatal(parked["decision"])
	}
	accepted := decide(t, bin, repo, home, parked, "accept", "accept-head", nil)
	if accepted["outcome"] != "ok" {
		t.Fatal(accepted)
	}
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	p := run["pipeline"].(map[string]any)
	review := stageOf(t, done, "review")
	acceptance := p["acceptances"].([]any)[0].(map[string]any)
	if review["status"] != "accepted-risk" || acceptance["candidate"] != run["candidate"] || acceptance["inputs"] != review["inputs"] || acceptance["actor"] != "axi" || len(acceptance["findings"].([]any)) != 1 {
		t.Fatalf("acceptance binding: %v %v", review, acceptance)
	}
	if run["status"] == "locally-ready" || stageOf(t, done, "test")["status"] != "failed" || p["outcome"] != nil {
		t.Fatalf("accepted review risk satisfied a failing test: %v", p)
	}

	bin, repo, home, root, v = authorityFixture(t, "auto", 1, strictPlan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	decide(t, bin, repo, home, waitPipeline(t, bin, repo, home), "accept", "accept-head", nil)
	ready := waitPipeline(t, bin, repo, home)
	if p := runOf(t, ready)["pipeline"].(map[string]any); runOf(t, ready)["status"] != "locally-ready" || p["outcome"] != "accepted-risk" {
		t.Fatalf("accepted head with passing gates is not marked accepted risk: %v", p)
	}
	changed := mutate(t, bin, repo, home, ready, "apply", "new-rules", map[string]any{"mode": "auto", "preset": "full", "limit": 1, "rules": []string{"unslop", "no-comments"}})
	if stageOf(t, changed, "review")["status"] != "stale" {
		t.Fatalf("acceptance survived a configuration change: %v", stageOf(t, changed, "review"))
	}
	mutate(t, bin, repo, home, changed, "pipeline", "continue", nil)
	again := waitPipeline(t, bin, repo, home)
	if decisionIn(t, again)["kind"] != "cap" || runOf(t, again)["status"] == "locally-ready" {
		t.Fatal("acceptance carried to a different configuration input")
	}
}

func TestFinalFixGrantsOneRepairAndFinalReviewAcrossRestart(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := authorityFixture(t, "auto", 1, repairPlan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("clean"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	granted := decide(t, bin, repo, home, parked, "final", "final-fix", nil)
	if granted["outcome"] != "ok" {
		t.Fatal(granted)
	}
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	p := run["pipeline"].(map[string]any)
	if run["status"] != "locally-ready" || p["outcome"] != "repaired-pass" {
		t.Fatalf("final-fix did not finish: %v", p)
	}
	b := budgetIn(t, done)
	if b["review"] != float64(1) || b["repair"] != float64(0) || b["final_fix_repair"] != float64(1) || b["final_fix_review"] != float64(1) {
		t.Fatalf("final-fix allowance was not separate from the cap: %v", b)
	}
	grant := p["final_fix"].(map[string]any)
	if grant["actor"] != "axi" || grant["review_used"] != float64(1) || grant["decision"] != parked["gate_id"] {
		t.Fatalf("grant provenance: %v", grant)
	}
	if replay := decide(t, bin, repo, home, parked, "final", "final-fix", nil); replay["token"] != granted["token"] {
		t.Fatalf("final-fix replay was not idempotent: %v", replay)
	}
	if again := decide(t, bin, repo, home, parked, "final-again", "final-fix", nil); again["outcome"] != "refused" {
		t.Fatal("second final-fix grant accepted")
	}
	stopDaemon(t, home)
	restarted := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	a, _ := json.Marshal(grant)
	c, _ := json.Marshal(runOf(t, restarted)["pipeline"].(map[string]any)["final_fix"])
	if string(a) != string(c) {
		t.Fatalf("grant changed across restart:\n%s\n%s", a, c)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["review"] != 1 {
		t.Fatalf("final-fix repaired more than once: %v", got)
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 2 {
		t.Fatalf("final review count: %v", got)
	}
}

func TestFinalFixWithoutRepairableFindingsRunsFinalReview(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := authorityFixture(t, "auto", 1, strictPlan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	os.Remove(filepath.Join(home, "fixer.json"))
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	decide(t, bin, repo, home, parked, "final", "final-fix", nil)
	again := waitPipeline(t, bin, repo, home)
	d := decisionIn(t, again)
	if d["kind"] != "cap" || has(strs(d["options"]), "final-fix") {
		t.Fatalf("second cap decision offered final-fix again: %v", d)
	}
	if b := budgetIn(t, again); b["final_fix_repair"] != float64(0) || b["final_fix_review"] != float64(1) || b["review"] != float64(1) {
		t.Fatalf("final review without repair: %v", b)
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 2 {
		t.Fatalf("final review did not run: %v", got)
	}
}

func TestRethinkLinksSuccessorAndPreservesEvidence(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, parked := capFixture(t, "auto", 3, repairPlan)
	old := runOf(t, parked)
	successor := decide(t, bin, repo, home, parked, "rethink", "rethink", nil)
	next := runOf(t, successor)
	if successor["outcome"] != "ok" || next["id"] == old["id"] || next["predecessor"] != old["id"] || next["status"] != "unconfigured" {
		t.Fatalf("rethink successor: %v", next)
	}
	if len(next["findings"].([]any)) != 1 || next["configs"].([]any)[0].(map[string]any)["settings"].(map[string]any)["limit"] != float64(3) {
		t.Fatalf("successor did not inherit findings and settings: %v", next)
	}
	q, _ := json.Marshal(map[string]any{"version": 1, "op": "inspect", "run_id": old["id"]})
	previous := runOf(t, axi(t, bin, repo, home, string(q)))
	pp := previous["pipeline"].(map[string]any)
	if previous["status"] != "rethought" || previous["successor"] != next["id"] || len(pp["repairs"].([]any)) != 1 || len(pp["reviews"].([]any)) != 1 || previous["review_used"] != float64(2) {
		t.Fatalf("old evidence not preserved: %v", previous)
	}
	if ref := strings.TrimSpace(git(t, repo, "rev-parse", pp["ref"].(string))); ref != old["candidate"] {
		t.Fatalf("rethink touched the old candidate ref: %s", ref)
	}
	target := old["target"]
	b, _ := json.Marshal(map[string]any{"version": 1, "op": "prepare", "run_id": successor["run_id"], "input_id": successor["input_id"], "expected": successor["token"], "gate_id": successor["gate_id"], "action_id": "prepare-successor", "intent": "Change the sample differently", "target": target})
	prepared := axi(t, bin, repo, home, string(b))
	if started := mutate(t, bin, repo, home, prepared, "pipeline", "successor-start", nil); started["outcome"] != "ok" {
		t.Fatalf("successor blocked by released custody: %v", started["error"])
	}
	waitPipeline(t, bin, repo, home)
	_ = root
}

func TestDiscardPreservesEvidenceAndCommits(t *testing.T) {
	t.Parallel()
	bin, repo, home, _, parked := capFixture(t, "auto", 3, repairPlan)
	old := runOf(t, parked)
	submitted := old["submitted"].(map[string]any)["head"]
	discarded := decide(t, bin, repo, home, parked, "discard", "discard", nil)
	run := runOf(t, discarded)
	p := run["pipeline"].(map[string]any)
	if discarded["outcome"] != "ok" || run["status"] != "discarded" || p["status"] != "discarded" || len(p["repairs"].([]any)) != 1 {
		t.Fatalf("discard: %v", run)
	}
	if ref := strings.TrimSpace(git(t, repo, "rev-parse", p["ref"].(string))); ref != old["candidate"] {
		t.Fatalf("discard deleted or moved the unpublished candidate: %s", ref)
	}
	if _, err := os.Stat(p["worktree"].(string)); err != nil {
		t.Fatalf("discard removed the candidate worktree: %v", err)
	}
	if head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")); head != submitted {
		t.Fatal("discard changed the source")
	}
	if has(strs(discarded["actions"]), "pipeline") || has(strs(discarded["actions"]), "decide") || !has(strs(discarded["actions"]), "new") {
		t.Fatalf("actions after discard: %v", discarded["actions"])
	}
	if next := mutate(t, bin, repo, home, discarded, "new", "after-discard", nil); next["outcome"] != "ok" {
		t.Fatalf("new run after discard: %v", next["error"])
	}
}

func yoloPlan(root string) map[string]any {
	plan := repairPlan(root)
	plan["yolo"] = map[string]any{"actor": "night-shift", "cap": "final-fix"}
	return plan
}

func TestYoloDelegatesOnlyOperationalAndCapChoices(t *testing.T) {
	t.Parallel()
	bin, repo, home, _, parked := capFixture(t, "yolo", 3, yoloPlan)
	run := runOf(t, parked)
	p := run["pipeline"].(map[string]any)
	if p["repairs"].([]any)[0].(map[string]any)["actor"] != "yolo:night-shift" {
		t.Fatalf("yolo finding repair actor: %v", p["repairs"])
	}
	first := p["decisions"].([]any)[0].(map[string]any)
	answer := first["answer"].(map[string]any)
	if first["kind"] != "cap" || answer["choice"] != "final-fix" || answer["actor"] != "yolo:night-shift" || answer["reason"] != "trusted pipeline.json yolo cap policy" {
		t.Fatalf("yolo cap choice provenance: %v", first)
	}
	if b := budgetIn(t, parked); b["final_fix_repair"] != float64(1) || b["final_fix_review"] != float64(1) || b["total"] != float64(3) {
		t.Fatalf("yolo final-fix accounting: %v", b)
	}
	d := decisionIn(t, parked)
	if d["kind"] != "cap" || has(strs(d["options"]), "final-fix") || run["status"] != "needs-decision" {
		t.Fatalf("yolo answered beyond its single final-fix: %v", d)
	}
	_, _, _ = bin, repo, home
}

func TestYoloWithoutActorParks(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := authorityFixture(t, "yolo", 6, repairPlan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	if d := decisionIn(t, parked); d["kind"] != "findings" || !strings.Contains(d["reason"].(string), "no configured actor") {
		t.Fatalf("yolo without a configured actor did not park: %v", d)
	}
}

func TestUserOnlyAuthorityParksInEveryMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ mode, file, value, category string }{
		{"ask", "fix-escalate", "intent", "intent"},
		{"auto", "fix-escalate", "acceptance", "acceptance"},
		{"yolo", "fix-escalate", "intent", "intent"},
		{"auto", "fix-content", "DELETE", "destructive"},
	} {
		t.Run(tc.mode+"-"+tc.category, func(t *testing.T) {
			t.Parallel()
			bin, repo, home, root, v := authorityFixture(t, tc.mode, 6, yoloPlan)
			os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
			os.WriteFile(filepath.Join(root, tc.file), []byte(tc.value), 0600)
			mutate(t, bin, repo, home, v, "pipeline", "start", nil)
			parked := waitPipeline(t, bin, repo, home)
			if tc.mode == "ask" {
				decide(t, bin, repo, home, parked, "authorize", "repair", nil)
				parked = waitPipeline(t, bin, repo, home)
			}
			d := decisionIn(t, parked)
			if d["kind"] != "authority" || d["user_only"] != true || d["category"] != tc.category || strings.Join(strs(d["options"]), ",") != "rethink,discard" {
				t.Fatalf("authority decision: %v", d)
			}
			repairs := runOf(t, parked)["pipeline"].(map[string]any)["repairs"].([]any)
			if last := repairs[len(repairs)-1].(map[string]any); last["status"] != "needs-authority" || last["head"] != "" {
				t.Fatalf("authority change was committed: %v", last)
			}
			if got := decide(t, bin, repo, home, parked, "budget", "increase-cap", map[string]any{"limit": 20}); got["outcome"] != "refused" {
				t.Fatal("a round budget granted user-only authority")
			}
			if got := decide(t, bin, repo, home, parked, "accept", "accept-head", nil); got["outcome"] != "refused" {
				t.Fatal("accept-head answered user-only authority")
			}
			if len(runOf(t, parked)["pipeline"].(map[string]any)["decisions"].([]any)) != map[bool]int{true: 2, false: 1}[tc.mode == "ask"] {
				t.Fatal("an automatic actor answered the authority decision")
			}
			if got := decide(t, bin, repo, home, parked, "discard", "discard", nil); got["outcome"] != "ok" || runOf(t, got)["status"] != "discarded" {
				t.Fatalf("explicit decision refused: %v", got)
			}
		})
	}
}

func TestLaterReviewReopensResolvedFinding(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := authorityFixture(t, "auto", 4, repairPlan)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	os.WriteFile(filepath.Join(root, "fix-then-review-review"), []byte("clean"), 0600)
	os.WriteFile(filepath.Join(root, "require-test"), []byte("gate fixed\n"), 0600)
	os.WriteFile(filepath.Join(root, "fix-content-test"), []byte("gate fixed\n"), 0600)
	os.WriteFile(filepath.Join(root, "fix-then-review-test"), []byte("findings"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	p := run["pipeline"].(map[string]any)
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["review"] != 1 || got["test"] != 1 {
		t.Fatalf("fixture did not reach the later review: %v", got)
	}
	if run["status"] == "locally-ready" {
		t.Fatalf("a finding reported again after its resolution did not block readiness: %v", p)
	}
	findings := run["findings"].([]any)
	if len(findings) != 1 || findings[0].(map[string]any)["head"] != run["candidate"] {
		t.Fatalf("finding ledger did not take the later report: %v", findings)
	}
	d := decisionIn(t, done)
	if d["kind"] != "cap" || !has(strs(d["findings"]), findings[0].(map[string]any)["id"].(string)) {
		t.Fatalf("reopened finding is not in the decision: %v", d)
	}
	resolutions := p["resolutions"].([]any)
	if len(resolutions) != 1 || resolutions[0].(map[string]any)["reopened"] != run["review"].(map[string]any)["id"] {
		t.Fatalf("resolution history lost or not reopened: %v", resolutions)
	}
}

func TestOnePassReviewUpdatesRetainedFindingData(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	mutate(t, bin, repo, home, v, "review", "first-review", nil)
	first := waitReview(t, bin, repo, home)
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "moved")
	next := mutate(t, bin, repo, home, first, "new", "next-run", nil)
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop", "no-comments"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	next = mutate(t, bin, repo, home, next, "apply", "restore-test-model", settings)
	head := runOf(t, next)["candidate"]
	q, _ := json.Marshal(map[string]any{"version": 1, "op": "prepare", "run_id": next["run_id"], "input_id": next["input_id"], "expected": next["token"], "gate_id": "none", "action_id": "prepare-next", "intent": "Review sample change", "target": runOf(t, first)["target"]})
	prepared := axi(t, bin, repo, home, string(q))
	prepared = mutate(t, bin, repo, home, prepared, "resolve-rules", "rules-next", nil)
	mutate(t, bin, repo, home, prepared, "review", "second-review", nil)
	done := waitReview(t, bin, repo, home)
	findings := runOf(t, done)["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("retained finding count %d, want 1", len(findings))
	}
	if got := findings[0].(map[string]any)["head"]; got != head {
		t.Fatalf("retained finding head %v, want %v", got, head)
	}
}

func TestParseSelection(t *testing.T) {
	t.Parallel()
	group := []string{"aaaa1111", "aaaa2222", "bbbb3333"}
	for _, tc := range []struct {
		line  string
		ids   []string
		whole bool
		err   string
	}{
		{line: "", whole: true},
		{line: "   ", err: "no finding ID"},
		{line: "\t", err: "no finding ID"},
		{line: "bbbb3333", ids: []string{"bbbb3333"}},
		{line: "aaaa1111 bbbb3333", ids: []string{"aaaa1111", "bbbb3333"}},
		{line: "aaaa1111,bbbb3333", ids: []string{"aaaa1111", "bbbb3333"}},
		{line: " aaaa2222 , bbbb3333 ", ids: []string{"aaaa2222", "bbbb3333"}},
		{line: ",", err: "no finding ID"},
		{line: ",,,", err: "no finding ID"},
		{line: " , ", err: "no finding ID"},
		{line: "zzz", err: "unknown finding ID: zzz"},
		{line: "bbbb", err: "unknown finding ID: bbbb"},
		{line: "aaaa", err: "unknown finding ID: aaaa"},
		{line: "BBBB3333", err: "unknown finding ID: BBBB3333"},
		{line: "bbbb3333 bbbb3333", err: "duplicate finding ID: bbbb3333"},
		{line: "aaaa1111, zzz", err: "unknown finding ID: zzz"},
	} {
		ids, whole, err := parseSelection(tc.line, group)
		got := ""
		if err != nil {
			got = err.Error()
		}
		if whole != tc.whole || strings.Join(ids, ",") != strings.Join(tc.ids, ",") || (tc.err == "") != (err == nil) || !strings.Contains(got, tc.err) {
			t.Errorf("parseSelection(%q) = %v, %v, %q; want %v, %v, %q", tc.line, ids, whole, got, tc.ids, tc.whole, tc.err)
		}
		if err != nil && ids != nil {
			t.Errorf("parseSelection(%q) returned IDs with an error", tc.line)
		}
	}
}

func TestParseTerminalNumbersAndSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text string
		want int
		ok   bool
	}{{"7", 7, true}, {"1000", 1000, true}, {"", 0, false}, {" 7", 0, false}, {"7 ", 0, false}, {"7x", 0, false}, {"+7", 0, false}, {"-1", 0, false}, {"07", 0, false}, {"99999999999999999999", 0, false}} {
		got, err := parseCount(tc.text)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("parseCount(%q) = %d, %v", tc.text, got, err)
		}
	}
	for _, tc := range []struct {
		text string
		ok   bool
	}{{"ask one-pass 7", true}, {"auto full 6", true}, {"ask one-pass", false}, {"ask one-pass 7 extra", false}, {"ask one-pass 7x", false}, {" ask one-pass 7", false}, {"ask  one-pass 7", false}} {
		mode, preset, limit, err := parseDraft(tc.text)
		if (err == nil) != tc.ok || tc.ok && mode+" "+preset+" "+strconv.Itoa(limit) != tc.text {
			t.Errorf("parseDraft(%q) = %q %q %d, %v", tc.text, mode, preset, limit, err)
		}
	}
}

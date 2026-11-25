package engine

import (
	"testing"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func TestMergeReviewPassesDeduplicatesOverlappingFindingsAndCoverage(t *testing.T) {
	passes := []reviewOutcome{
		{Status: "partial-review", Result: &Verdict{Verdict: "partial", Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{{ID: "first", Priority: "P1", File: "a.go", Start: 4, End: 6, Explanation: "first explanation", Action: "first action"}}, Uncovered: []UncoveredPath{{Path: "b.go", Reason: "budget"}}}, Answer: agentAnswer{Usage: TokenUsage{Tokens: 10, CostUSD: 1}}},
		{Status: "outstanding-findings", Result: &Verdict{Verdict: "findings", Head: "head", Coverage: []string{"b.go"}, Findings: []Finding{{ID: "second", Priority: "P1", File: "a.go", Start: 6, End: 8, Explanation: "second explanation", Action: "second action"}}}, Answer: agentAnswer{Usage: TokenUsage{Tokens: 20, CostUSD: 2}}},
	}

	got := mergeReviewPasses(passes, []string{"a.go", "b.go"})
	if got.Status != "outstanding-findings" || got.Result.Verdict != "findings" {
		t.Fatalf("merged verdict = %s/%s, want outstanding findings", got.Status, got.Result.Verdict)
	}
	if len(got.Result.Coverage) != 2 || len(got.Result.Uncovered) != 0 {
		t.Fatalf("coverage = %#v, uncovered = %#v", got.Result.Coverage, got.Result.Uncovered)
	}
	if len(got.Result.Findings) != 1 || got.Result.Findings[0].ID != "first" {
		t.Fatalf("merged findings = %#v, want first overlapping finding only", got.Result.Findings)
	}
	if got.Answer.Usage.Tokens != 30 || got.Answer.Usage.CostUSD != 3 {
		t.Fatalf("usage = %#v, want summed usage", got.Answer.Usage)
	}
}

func TestMergeReviewPassesPreservesSinglePassResult(t *testing.T) {
	pass := reviewOutcome{Status: "clean-review", Result: &Verdict{Verdict: "clean", Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{}}, Answer: agentAnswer{Usage: TokenUsage{Tokens: 7}}}
	got := mergeReviewPasses([]reviewOutcome{pass}, []string{"a.go"})
	if got.Status != pass.Status || got.Result.Verdict != pass.Result.Verdict || len(got.Result.Coverage) != 1 || got.Answer.Usage.Tokens != 7 {
		t.Fatalf("single pass changed: %#v", got)
	}
}

func TestMergeReviewPassesKeepsSeparateRanges(t *testing.T) {
	passes := []reviewOutcome{{Status: "outstanding-findings", Result: &Verdict{Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{{ID: "one", File: "a.go", Start: 1, End: 2}, {ID: "two", File: "a.go", Start: 8, End: 9}}}}}
	got := mergeReviewPasses(passes, []string{"a.go"})
	if len(got.Result.Findings) != 2 {
		t.Fatalf("findings = %#v, want non-overlapping findings retained", got.Result.Findings)
	}
}

func TestMergeReviewPassesKeepsRecordsAndEvidence(t *testing.T) {
	confinement := &runner.Confinement{}
	passes := []reviewOutcome{
		{Status: "clean-review", Snapshot: "unchanged", Confinement: confinement, Result: &Verdict{Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{}, ReportedModel: "m", Documentation: &DocAssessment{Status: "documented", Reason: "ok"}, Resolutions: []ResolutionCheck{{Finding: "f1", Check: "confirmed", Reason: "fixed"}}, Dismissed: []Finding{{ID: "d1"}}, DismissalReasons: map[string]string{"d1": "noise"}}},
		{Status: "clean-review", Snapshot: "unchanged", Result: &Verdict{Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{}, Documentation: &DocAssessment{Status: "missing", Reason: "no docs"}, Resolutions: []ResolutionCheck{{Finding: "f1", Check: "rejected", Reason: "still broken"}}}},
	}
	got := mergeReviewPasses(passes, []string{"a.go"})
	v := got.Result
	if v.Documentation == nil || v.Documentation.Status != "missing" || len(v.Resolutions) != 1 || v.Resolutions[0].Check != "rejected" || len(v.Dismissed) != 1 || v.DismissalReasons["d1"] != "noise" || v.ReportedModel != "m" {
		t.Fatalf("records not merged: %#v", v)
	}
	if got.Snapshot != "unchanged" || got.Confinement != confinement {
		t.Fatalf("evidence lost: snapshot %q confinement %v", got.Snapshot, got.Confinement)
	}
}

func TestMergeReviewPassesUsesSelectedPathsAndKeepsReasons(t *testing.T) {
	pass := reviewOutcome{Status: "clean-review", Result: &Verdict{Head: "head", Coverage: []string{"b.go"}, Findings: []Finding{}, Uncovered: []UncoveredPath{{Path: "gen.go", Reason: "generated"}}}}
	got := mergeReviewPasses([]reviewOutcome{pass, pass}, []string{"b.go", "gen.go"})
	if len(got.Result.Uncovered) != 1 || got.Result.Uncovered[0].Reason != "generated" {
		t.Fatalf("uncovered = %#v", got.Result.Uncovered)
	}
}

func TestMergeReviewPassesModifiedSnapshotIsInconclusive(t *testing.T) {
	ok := reviewOutcome{Status: "clean-review", Snapshot: "unchanged", Result: &Verdict{Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{}}}
	bad := reviewOutcome{Status: "inconclusive", Snapshot: "modified: x"}
	got := mergeReviewPasses([]reviewOutcome{ok, bad}, []string{"a.go"})
	if got.Status != "inconclusive" || got.Result != nil || got.Snapshot != "modified: x" {
		t.Fatalf("got %#v", got)
	}
}

func TestMergeReviewPassesCollapsesBridgingFinding(t *testing.T) {
	passes := []reviewOutcome{
		{Status: "outstanding-findings", Result: &Verdict{Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{{ID: "one", Priority: "P2", File: "a.go", Start: 1, End: 5}, {ID: "two", Priority: "P1", File: "a.go", Start: 10, End: 15}}}},
		{Status: "outstanding-findings", Result: &Verdict{Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{{ID: "bridge", Priority: "P2", File: "a.go", Start: 4, End: 12}}}},
	}
	got := mergeReviewPasses(passes, []string{"a.go"})
	f := got.Result.Findings
	if len(f) != 1 || f[0].Start != 1 || f[0].End != 15 || f[0].ID != "two" {
		t.Fatalf("findings = %#v, want one finding 1-15 keeping the P1", f)
	}
}

func TestMergeReviewPassesDropsUncoveredAnotherPassCovered(t *testing.T) {
	first := reviewOutcome{Status: "partial-review", Result: &Verdict{Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{}, Uncovered: []UncoveredPath{{Path: "b.go", Reason: "not covered by reviewer within budget"}}}}
	second := reviewOutcome{Status: "clean-review", Result: &Verdict{Head: "head", Coverage: []string{"b.go"}, Findings: []Finding{}}}
	got := mergeReviewPasses([]reviewOutcome{first, second}, []string{"a.go", "b.go"})
	if got.Status != "clean-review" || len(got.Result.Uncovered) != 0 {
		t.Fatalf("status %s uncovered %#v", got.Status, got.Result.Uncovered)
	}
}

func TestMergeReviewPassesAllInconclusiveStaysInconclusive(t *testing.T) {
	pass := reviewOutcome{Status: "inconclusive", Result: &Verdict{Verdict: "inconclusive", Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{}}}
	got := mergeReviewPasses([]reviewOutcome{pass, pass}, []string{"a.go"})
	if got.Status != "inconclusive" || got.Result.Verdict != "inconclusive" {
		t.Fatalf("status %s verdict %s, want inconclusive", got.Status, got.Result.Verdict)
	}
}

func TestMergeReviewPassesIgnoresCoverageOfInconclusivePass(t *testing.T) {
	bad := reviewOutcome{Status: "inconclusive", Result: &Verdict{Verdict: "inconclusive", Head: "head", Coverage: []string{"a.go", "b.go"}, Findings: []Finding{}}}
	good := reviewOutcome{Status: "clean-review", Result: &Verdict{Verdict: "clean", Head: "head", Coverage: []string{"a.go"}, Findings: []Finding{}}}
	got := mergeReviewPasses([]reviewOutcome{bad, good}, []string{"a.go", "b.go"})
	if got.Status != "partial-review" || len(got.Result.Uncovered) != 1 || got.Result.Uncovered[0].Path != "b.go" {
		t.Fatalf("status %s uncovered %#v", got.Status, got.Result.Uncovered)
	}
}

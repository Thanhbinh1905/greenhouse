package engine

import "testing"

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

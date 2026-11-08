package main

import (
	"testing"
)

func bugCase() Case {
	return Case{ID: "bug", Category: "bug",
		Expected: []Expected{
			{File: "a.go", Start: 10, End: 12, Origin: "correctness", Severity: "high", Priorities: []string{"P0", "P1"}},
			{File: "a.go", Start: 20, End: 20, Origin: "policy", Rule: "no-comments/necessary-comments", Severity: "low", Priorities: []string{"P2", "P3"}},
		},
		Noise: []Noise{{File: "a.go", Start: 30, End: 32}},
	}
}

func TestScoreCaseMatchesFileRangeOriginAndRule(t *testing.T) {
	s := ScoreCase(bugCase(), "outstanding-findings", []Finding{
		{Priority: "P1", Origin: "correctness", File: "a.go", Start: 12, End: 14},
		{Priority: "P1", Origin: "correctness", File: "a.go", Start: 11, End: 11},
		{Priority: "P3", Origin: "policy", Rule: "unslop/plain-prose", File: "a.go", Start: 20, End: 20},
		{Priority: "P2", Origin: "correctness", File: "b.go", Start: 10, End: 12},
		{Priority: "P0", Origin: "correctness", File: "a.go", Start: 31, End: 31},
	})
	if s.Expected != 2 || s.Detected != 1 || s.HighExpected != 1 || s.HighDetected != 1 {
		t.Fatalf("detection = %+v", s)
	}
	if s.Findings != 5 || s.Matched != 2 || s.Duplicates != 1 || s.FalsePositives != 3 || s.NoiseFlagged != 1 || s.Blocking != 1 {
		t.Fatalf("finding accounting = %+v", s)
	}
	if s.PriorityJudged != 1 || s.PriorityCorrect != 1 {
		t.Fatalf("priority = %+v", s)
	}
	if len(s.Missed) != 1 || s.Missed[0].Start != 20 {
		t.Fatalf("missed = %+v", s.Missed)
	}
}

func TestScoreCaseRejectsOversizedRanges(t *testing.T) {
	s := ScoreCase(bugCase(), "outstanding-findings", []Finding{{Priority: "P1", Origin: "correctness", File: "a.go", Start: 1, End: 1 + maxSpan}})
	if s.Detected != 0 || s.FalsePositives != 1 {
		t.Fatalf("a range of %d lines must not match: %+v", maxSpan+1, s)
	}
	s = ScoreCase(bugCase(), "outstanding-findings", []Finding{{Priority: "P1", Origin: "correctness", File: "a.go", Start: 1, End: maxSpan}})
	if s.Detected != 1 {
		t.Fatalf("a range of %d lines overlapping the defect must match: %+v", maxSpan, s)
	}
}

func TestScoreCaseCountsWrongPriority(t *testing.T) {
	s := ScoreCase(bugCase(), "outstanding-findings", []Finding{{Priority: "P2", Origin: "correctness", File: "a.go", Start: 10, End: 10}})
	if s.Detected != 1 || s.PriorityJudged != 1 || s.PriorityCorrect != 0 {
		t.Fatalf("score = %+v", s)
	}
}

func TestScoreCaseCleanAndInconclusive(t *testing.T) {
	clean := Case{ID: "clean", Category: "clean"}
	s := ScoreCase(clean, "clean-review", nil)
	if s.Blocking != 0 || s.FalsePositives != 0 {
		t.Fatalf("clean = %+v", s)
	}
	s = ScoreCase(clean, "outstanding-findings", []Finding{{Priority: "P3", Origin: "policy", File: "a.go", Start: 1, End: 1}, {Priority: "P0", Origin: "correctness", File: "a.go", Start: 2, End: 2}})
	if s.Blocking != 1 || s.FalsePositives != 2 {
		t.Fatalf("clean with findings = %+v", s)
	}
	s = ScoreCase(clean, "inconclusive", nil)
	if !s.Inconclusive || s.Blocking != 1 {
		t.Fatalf("an inconclusive clean review blocks: %+v", s)
	}
	s = ScoreCase(bugCase(), "inconclusive", nil)
	if !s.Inconclusive || s.Detected != 0 || len(s.Missed) != 2 {
		t.Fatalf("inconclusive bug = %+v", s)
	}
}

func TestSummarizeComputesRatesAndBar(t *testing.T) {
	scores := []CaseScore{
		ScoreCase(bugCase(), "outstanding-findings", []Finding{
			{Priority: "P1", Origin: "correctness", File: "a.go", Start: 10, End: 10},
			{Priority: "P2", Origin: "correctness", File: "a.go", Start: 40, End: 40},
		}),
		ScoreCase(Case{ID: "clean", Category: "clean"}, "clean-review", nil),
		ScoreCase(Case{ID: "noise", Category: "noise", Noise: []Noise{{File: "n.go", Start: 1, End: 3}}}, "outstanding-findings", []Finding{{Priority: "P3", Origin: "policy", File: "n.go", Start: 2, End: 2}}),
	}
	m := Summarize(scores)
	if m.Cases != 3 || m.Expected != 2 || m.Detected != 1 || m.HighExpected != 1 || m.HighDetected != 1 {
		t.Fatalf("summary counts = %+v", m)
	}
	if *m.Recall != 0.5 || *m.HighRecall != 1 || *m.Precision != 1.0/3 || *m.PriorityAccuracy != 1 {
		t.Fatalf("summary rates = recall %v high %v precision %v priority %v", *m.Recall, *m.HighRecall, *m.Precision, *m.PriorityAccuracy)
	}
	if m.FalsePositives != 2 || m.NoiseFalsePositives != 1 || m.CleanFalsePositives != 0 || m.CleanBlocking != 0 || !m.BarMet {
		t.Fatalf("summary bar = %+v", m)
	}
	scores[1] = ScoreCase(Case{ID: "clean", Category: "clean"}, "outstanding-findings", []Finding{{Priority: "P1", Origin: "correctness", File: "c.go", Start: 1, End: 1}})
	if m = Summarize(scores); m.CleanBlocking != 1 || m.BarMet {
		t.Fatalf("clean blocking must fail the bar: %+v", m)
	}
	if m = Summarize([]CaseScore{ScoreCase(Case{ID: "clean", Category: "clean"}, "clean-review", nil)}); m.Precision != nil || m.HighRecall != nil || m.BarMet {
		t.Fatalf("undefined rates must be nil and fail the bar: %+v", m)
	}
}

package main

import (
	"strings"
	"testing"
)

func bugCase() Case {
	return Case{ID: "bug", Category: "bug",
		Expected: []Expected{
			{ID: "loop-bound", File: "a.go", Start: 10, End: 12, Origin: "correctness", Severity: "high", Priorities: []string{"P0", "P1"}},
			{ID: "narrating-comment", File: "a.go", Start: 20, End: 20, Origin: "policy", Rule: "no-comments/necessary-comments", Severity: "low", Priorities: []string{"P2", "P3"}},
		},
		Noise: []Noise{{File: "a.go", Start: 30, End: 32}},
	}
}

func record(k Case, findings []Finding, n int, expected, verdict string) Record {
	f := findings[n]
	return Record{Case: k.ID, Finding: n, FindingDigest: findingDigest(f), Expected: expected, Verdict: verdict, Rationale: `The finding says "` + f.Explanation + `".`}
}

func mustScore(t *testing.T, k Case, status string, findings []Finding, records []Record) CaseScore {
	t.Helper()
	s, err := ScoreCase(k, status, findings, records)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestScoreCaseCreditsOnlyAdjudicatedEligibleFindings(t *testing.T) {
	k := bugCase()
	findings := []Finding{
		{Priority: "P1", Origin: "correctness", File: "a.go", Start: 12, End: 14, Explanation: "The loop reads values past the end."},
		{Priority: "P1", Origin: "correctness", File: "a.go", Start: 11, End: 11, Explanation: "Index len(values) is out of range."},
		{Priority: "P3", Origin: "policy", Rule: "unslop/plain-prose", File: "a.go", Start: 20, End: 20, Explanation: "Inflated wording here."},
		{Priority: "P2", Origin: "correctness", File: "b.go", Start: 10, End: 12, Explanation: "Unrelated file."},
		{Priority: "P0", Origin: "correctness", File: "a.go", Start: 31, End: 31, Explanation: "Legacy bug in noise."},
		{Priority: "P3", Origin: "policy", Rule: "no-comments/necessary-comments", File: "a.go", Start: 20, End: 20, Explanation: "This comment narrates code."},
	}
	s := mustScore(t, k, "outstanding-findings", findings, []Record{record(k, findings, 0, "loop-bound", "detects"), record(k, findings, 1, "loop-bound", "duplicate")})
	if s.Expected != 2 || s.Detected != 1 || s.HighDetected != 1 || s.Matched != 2 || s.Duplicates != 1 {
		t.Fatalf("detection = %+v", s)
	}
	if s.FalsePositives != 4 || s.Unadjudicated != 1 || s.NoiseFlagged != 1 || s.Blocking != 1 || s.PriorityJudged != 1 || s.PriorityCorrect != 1 {
		t.Fatalf("an eligible finding without a record must not be credited: %+v", s)
	}
	verdicts := []string{}
	for _, a := range s.Adjudications {
		verdicts = append(verdicts, a.Verdict)
	}
	if strings.Join(verdicts, ",") != "detects,duplicate,false-positive,false-positive,false-positive,unadjudicated" {
		t.Fatalf("adjudications = %v", verdicts)
	}
}

func TestScoreCaseRejectsUnrelatedOverlappingFindingsSharingIdentifiers(t *testing.T) {
	k := bugCase()
	findings := []Finding{
		{Priority: "P1", Origin: "correctness", File: "a.go", Start: 10, End: 12, Explanation: "values and len(values) are read by concurrent callers without a lock, so the loop races."},
		{Priority: "P1", Origin: "correctness", File: "a.go", Start: 11, End: 11, Explanation: "The loop reads values[len(values)], past the end."},
		{Priority: "P2", Origin: "correctness", File: "a.go", Start: 10, End: 14, Explanation: "A negative n makes len(values) - n exceed len(values), past the end of the requested window."},
	}
	records := []Record{
		record(k, findings, 0, "loop-bound", "different-defect"),
		record(k, findings, 1, "loop-bound", "detects"),
		record(k, findings, 2, "loop-bound", "different-defect"),
	}
	s := mustScore(t, k, "outstanding-findings", findings, records)
	if s.Detected != 1 || s.Duplicates != 0 || s.WrongDefect != 2 || s.FalsePositives != 2 || s.Matched != 1 || s.Blocking != 1 {
		t.Fatalf("overlapping findings with shared identifiers must not detect or duplicate: %+v", s)
	}
	if s.Adjudications[0].Verdict != "different-defect" || s.Adjudications[1].Verdict != "detects" || s.Adjudications[2].Verdict != "different-defect" {
		t.Fatalf("adjudications = %+v", s.Adjudications)
	}
}

func TestScoreCaseRefusesInconsistentRecords(t *testing.T) {
	k := bugCase()
	findings := []Finding{
		{Priority: "P1", Origin: "correctness", File: "a.go", Start: 10, End: 10, Explanation: "Index out of range at the loop bound."},
		{Priority: "P1", Origin: "correctness", File: "a.go", Start: 11, End: 11, Explanation: "The same loop bound reads past the end."},
		{Priority: "P2", Origin: "correctness", File: "b.go", Start: 1, End: 1, Explanation: "Unrelated file."},
	}
	ok := record(k, findings, 0, "loop-bound", "detects")
	stale := ok
	stale.FindingDigest = "0"
	uncited := ok
	uncited.Rationale = "It is the loop bound bug."
	tests := map[string][]Record{
		"changed finding text":           {stale},
		"rationale without a quote":      {uncited},
		"ineligible finding":             {record(k, findings, 2, "loop-bound", "detects")},
		"expected not at the location":   {record(k, findings, 0, "narrating-comment", "detects")},
		"duplicate before detection":     {record(k, findings, 1, "loop-bound", "duplicate")},
		"second detection":               {ok, record(k, findings, 1, "loop-bound", "detects")},
		"two records for one finding":    {ok, ok},
		"unknown verdict":                {record(k, findings, 0, "loop-bound", "maybe")},
		"finding index outside the case": {{Case: "bug", Finding: 9, Expected: "loop-bound", Verdict: "detects"}},
		"record for another case":        {{Case: "other", Finding: 0, FindingDigest: ok.FindingDigest, Expected: "loop-bound", Verdict: "detects", Rationale: ok.Rationale}},
	}
	for name, records := range tests {
		if _, err := ScoreCase(k, "outstanding-findings", findings, records); err == nil {
			t.Errorf("%s: accepted %+v", name, records)
		}
	}
}

func TestScoreCaseRejectsOversizedRanges(t *testing.T) {
	k := bugCase()
	wide := []Finding{{Priority: "P1", Origin: "correctness", File: "a.go", Start: 1, End: 1 + maxSpan, Explanation: "Index out of range."}}
	if _, err := ScoreCase(k, "outstanding-findings", wide, []Record{record(k, wide, 0, "loop-bound", "detects")}); err == nil {
		t.Fatalf("a range of %d lines is not eligible", maxSpan+1)
	}
	s := mustScore(t, k, "outstanding-findings", wide, nil)
	if s.Detected != 0 || s.FalsePositives != 1 || s.Unadjudicated != 0 {
		t.Fatalf("score = %+v", s)
	}
	fit := []Finding{{Priority: "P1", Origin: "correctness", File: "a.go", Start: 1, End: maxSpan, Explanation: "Index out of range."}}
	if s = mustScore(t, k, "outstanding-findings", fit, []Record{record(k, fit, 0, "loop-bound", "detects")}); s.Detected != 1 {
		t.Fatalf("a range of %d lines overlapping the defect is eligible: %+v", maxSpan, s)
	}
}

func TestScoreCaseCountsWrongPriority(t *testing.T) {
	k := bugCase()
	findings := []Finding{{Priority: "P2", Origin: "correctness", File: "a.go", Start: 10, End: 10, Explanation: "Index out of range."}}
	s := mustScore(t, k, "outstanding-findings", findings, []Record{record(k, findings, 0, "loop-bound", "detects")})
	if s.Detected != 1 || s.PriorityJudged != 1 || s.PriorityCorrect != 0 {
		t.Fatalf("score = %+v", s)
	}
}

func TestScoreCaseCleanAndInconclusive(t *testing.T) {
	clean := Case{ID: "clean", Category: "clean"}
	s := mustScore(t, clean, "clean-review", nil, nil)
	if s.Blocking != 0 || s.FalsePositives != 0 {
		t.Fatalf("clean = %+v", s)
	}
	s = mustScore(t, clean, "outstanding-findings", []Finding{{Priority: "P3", Origin: "policy", File: "a.go", Start: 1, End: 1}, {Priority: "P0", Origin: "correctness", File: "a.go", Start: 2, End: 2}}, nil)
	if s.Blocking != 1 || s.FalsePositives != 2 {
		t.Fatalf("clean with findings = %+v", s)
	}
	s = mustScore(t, clean, "inconclusive", nil, nil)
	if !s.Inconclusive || s.Blocking != 1 {
		t.Fatalf("an inconclusive clean review blocks: %+v", s)
	}
	s = mustScore(t, bugCase(), "inconclusive", nil, nil)
	if !s.Inconclusive || s.Detected != 0 || len(s.Missed) != 2 {
		t.Fatalf("inconclusive bug = %+v", s)
	}
}

func TestSummarizeComputesRatesAndBar(t *testing.T) {
	k := bugCase()
	findings := []Finding{
		{Priority: "P1", Origin: "correctness", File: "a.go", Start: 10, End: 10, Explanation: "Index out of range."},
		{Priority: "P2", Origin: "correctness", File: "a.go", Start: 40, End: 40, Explanation: "Elsewhere."},
	}
	scores := []CaseScore{
		mustScore(t, k, "outstanding-findings", findings, []Record{record(k, findings, 0, "loop-bound", "detects")}),
		mustScore(t, Case{ID: "clean", Category: "clean"}, "clean-review", nil, nil),
		mustScore(t, Case{ID: "noise", Category: "noise", Noise: []Noise{{File: "n.go", Start: 1, End: 3}}}, "outstanding-findings", []Finding{{Priority: "P3", Origin: "policy", File: "n.go", Start: 2, End: 2}}, nil),
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
	scores[1] = mustScore(t, Case{ID: "clean", Category: "clean"}, "outstanding-findings", []Finding{{Priority: "P1", Origin: "correctness", File: "c.go", Start: 1, End: 1}}, nil)
	if m = Summarize(scores); m.CleanBlocking != 1 || m.BarMet {
		t.Fatalf("clean blocking must fail the bar: %+v", m)
	}
	if m = Summarize([]CaseScore{mustScore(t, Case{ID: "clean", Category: "clean"}, "clean-review", nil, nil)}); m.Precision != nil || m.HighRecall != nil || m.BarMet {
		t.Fatalf("undefined rates must be nil and fail the bar: %+v", m)
	}
}

func TestRescoreReportReadsAdjudications(t *testing.T) {
	k := bugCase()
	findings := []Finding{{Priority: "P1", Origin: "correctness", File: "a.go", Start: 10, End: 10, Explanation: "A negative size panics."}}
	c := &Corpus{Revision: 7, Digest: "d", Cases: []Case{k}}
	r := &Report{Config: RunConfig{CorpusRevision: 1}, Cases: []CaseRecord{{ID: "bug", Status: "outstanding-findings", Findings: findings, Score: CaseScore{Detected: 1}}}}
	if err := rescoreReport(r, c, nil); err != nil {
		t.Fatal(err)
	}
	if r.Config.ScoredRevision != 7 || r.Config.ScoredDigest != "d" || r.Cases[0].Score.Detected != 0 || r.Cases[0].Score.Unadjudicated != 1 || r.Summary.Unadjudicated != 1 {
		t.Fatalf("missing records must not be credited: %+v %+v", r.Config, r.Cases[0].Score)
	}
	a := &Adjudications{Schema: adjudicationSchema, Judge: Judge{Kind: "manual", Note: "test"}, Records: []Record{record(k, findings, 0, "loop-bound", "detects")}}
	if err := rescoreReport(r, c, a); err != nil {
		t.Fatal(err)
	}
	if r.Cases[0].Score.Detected != 1 || r.Summary.HighDetected != 1 {
		t.Fatalf("rescored = %+v", r.Cases[0].Score)
	}
	a.Records[0].Case = "gone"
	if err := rescoreReport(r, c, a); err == nil {
		t.Fatal("a record for a case outside the report must refuse")
	}
	r.Cases[0].ID = "gone"
	if err := rescoreReport(r, c, nil); err == nil {
		t.Fatal("a case missing from the corpus must refuse")
	}
}

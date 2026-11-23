package engine

import (
	"strings"
	"testing"
)

func TestIncompleteCoverageAndUnsettledResponse(t *testing.T) {
	t.Parallel()
	x := &Execution{Config: ReviewerConfig{Model: "fixture/model"}, Rules: InputIdentity{Candidate: "head"}, Paths: []string{"f"}}
	for _, body := range []string{
		`{"head":"other","verdict":"clean","coverage":["f"],"findings":[]}`,
		`{"head":"head","verdict":"clean","coverage":["f","f"],"findings":[]}`,
		`{"head":"head","verdict":"clean","coverage":["f"],"findings":null}`,
	} {
		if _, err := parseReview(verdictEvents(t, body), x); err == nil {
			t.Fatal(body)
		}
	}
	v, err := parseReview(verdictEvents(t, `{"head":"head","verdict":"clean","coverage":[],"findings":[]}`), x)
	if err != nil || v.Verdict != "partial" || len(v.Uncovered) != 1 || v.Uncovered[0].Path != "f" || v.Uncovered[0].Reason != "not covered by reviewer within budget" {
		t.Fatalf("missing coverage did not become partial: verdict=%+v err=%v", v, err)
	}
	events := verdictEvents(t, `{"head":"head","verdict":"clean","coverage":["f"],"findings":[]}`)
	events = []byte(strings.Replace(string(events), `"aborted":false`, `"aborted":true`, 1))
	if _, err := parseReview(events, x); err == nil {
		t.Fatal("aborted response accepted")
	}
}
func TestCleanReviewAndMissingSettledOutcome(t *testing.T) {
	t.Parallel()
	x := &Execution{Config: ReviewerConfig{Model: "fixture/model"}, Rules: InputIdentity{Candidate: "head"}, Paths: []string{"f"}}
	events := verdictEvents(t, `{"head":"head","verdict":"clean","coverage":["f"],"findings":[]}`)
	v, err := parseReview(events, x)
	if err != nil || v.Verdict != "clean" {
		t.Fatal(v, err)
	}
	events = []byte(strings.Replace(string(events), `,"aborted":false`, "", 1))
	if _, err := parseReview(events, x); err == nil {
		t.Fatal("missing settled outcome accepted")
	}
}
func TestChangedHunkRanges(t *testing.T) {
	t.Parallel()
	spans := changedRanges("--- a/sample\n+++ b/sample\n@@ -1,2 +1,3 @@\n context\n+++ misleading\n context\n@@ -10 +11 @@\n-old\n+new\n")
	if len(spans["sample"]) != 2 || spans["sample"][0] != [2]int{1, 3} || spans["sample"][1] != [2]int{11, 11} {
		t.Fatal(spans)
	}
}

func TestDeletionRangesUseCandidateAnchors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, candidate, hunk string
		want                  [2]int
	}{
		{"middle", "b/sample", "@@ -21,60 +20,0 @@", [2]int{20, 20}},
		{"beginning", "b/sample", "@@ -1,60 +0,0 @@", [2]int{1, 1}},
		{"end", "b/sample", "@@ -141,60 +140,0 @@", [2]int{140, 140}},
		{"whole file", "/dev/null", "@@ -1,60 +0,0 @@", [2]int{1, 60}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := changedRanges("--- a/sample\n+++ " + c.candidate + "\n" + c.hunk + "\n")
			if len(got["sample"]) != 1 || got["sample"][0] != c.want {
				t.Fatalf("deletion range = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDeletionContextUsesCandidateWindow(t *testing.T) {
	t.Parallel()
	var numbered strings.Builder
	for i := 0; i < 140; i++ {
		numbered.WriteString("candidate line\n")
	}
	ranges := changedRanges("--- a/sample\n+++ b/sample\n@@ -21,60 +20,0 @@\n")
	got := hunkContext([]reviewFile{{Path: "sample", Lines: numbered.String()}}, ranges)
	if n := strings.Count(got[0].Lines, "candidate line\n"); n != 40 {
		t.Fatalf("deletion context contains %d candidate lines, want window 1..40", n)
	}
}

func TestHunkContextPreservesNumberedWindows(t *testing.T) {
	t.Parallel()
	files := []reviewFile{{Path: "sample", Lines: "...\n2499: before\n2500: changed\n2501: after\n...\n"}}
	got := hunkContext(files, map[string][][2]int{"sample": {{2500, 2500}}})
	if !strings.Contains(got[0].Lines, "2500: changed\n") {
		t.Fatalf("candidate window lost its changed line: %q", got[0].Lines)
	}
}

package engine

import (
	"strings"
	"testing"
)

func TestIncompleteCoverageAndUnsettledResponse(t *testing.T) {
	t.Parallel()
	x := &Execution{Rules: InputIdentity{Candidate: "head"}, Paths: []string{"f"}}
	for _, body := range []string{
		`{"head":"head","verdict":"clean","coverage":[],"findings":[]}`,
		`{"head":"other","verdict":"clean","coverage":["f"],"findings":[]}`,
		`{"head":"head","verdict":"clean","coverage":["f","f"],"findings":[]}`,
		`{"head":"head","verdict":"clean","coverage":["f"],"findings":null}`,
	} {
		if _, err := parseReview(verdictEvents(t, body), x); err == nil {
			t.Fatal(body)
		}
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

package main

import (
	"strings"
	"testing"
)

func TestIncompleteCoverageAndUnsettledResponse(t *testing.T) {
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
func TestChangedHunkRanges(t *testing.T) {
	spans := changedRanges("--- a/sample\n+++ b/sample\n@@ -1,2 +1,3 @@\n context\n+changed\n context\n")
	if len(spans["sample"]) != 1 || spans["sample"][0] != [2]int{1, 3} {
		t.Fatal(spans)
	}
}

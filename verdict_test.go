package main

import (
	"encoding/json"
	"testing"
)

func verdictEvents(t *testing.T, text string) []byte {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "provider": "fixture", "model": "model", "stopReason": "stop", "content": []any{map[string]any{"type": "text", "text": text}}}})
	return append(b, []byte("\n{\"type\":\"agent_settled\",\"aborted\":false}\n")...)
}
func TestVerdictRejectsDuplicateAndCaseFoldedFields(t *testing.T) {
	x := &Execution{Rules: InputIdentity{Candidate: "head"}, Paths: []string{"f"}}
	for _, text := range []string{
		`{"head":"head","verdict":"findings","verdict":"clean","coverage":["f"],"findings":[]}`,
		`{"Head":"head","verdict":"clean","coverage":["f"],"findings":[]}`,
	} {
		if _, err := parseReview(verdictEvents(t, text), x); err == nil {
			t.Fatalf("accepted ambiguous verdict: %s", text)
		}
	}
}
func TestVerdictAcceptsPiSystemMessageWithStringContent(t *testing.T) {
	x := &Execution{Rules: InputIdentity{Candidate: "head"}, Paths: []string{"f"}}
	system := "{\"type\":\"message_start\",\"message\":{\"role\":\"system\",\"content\":\"\"}}\n{\"type\":\"message_end\",\"message\":{\"role\":\"system\",\"content\":\"\"}}\n"
	events := append([]byte(system), verdictEvents(t, `{"head":"head","verdict":"clean","coverage":["f"],"findings":[]}`)...)
	if v, err := parseReview(events, x); err != nil || v.Verdict != "clean" {
		t.Fatalf("Pi 1.5.0 system message must not invalidate the review: %v %v", v, err)
	}
}

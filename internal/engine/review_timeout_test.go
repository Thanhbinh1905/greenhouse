package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTimeoutReviewMetadata(t *testing.T) {
	for _, c := range []struct {
		reason string
		err    error
	}{{"timeout", context.DeadlineExceeded}, {"cancelled", context.Canceled}} {
		t.Run(c.reason, func(t *testing.T) {
			x := &Execution{ID: "review", Config: ReviewerConfig{Backend: "pi"}}
			stdout := []byte(`{"type":"message_end","message":{"role":"assistant","model":"reported","usage":{"input":100,"output":20,"totalTokens":120,"cost":{"total":0.5}}}}`)
			done := reviewFrom(x, -1, outputBuffer(stdout, 65536), outputBuffer(nil, 65536), nil, c.err)
			retainReviewMetadata(x, done)
			r := &Run{Host: &Host{Name: "host", ID: "1"}, Review: x}
			recordAttempt(r, Attempt{Execution: x.ID, Kind: "review"})
			var stored Run
			if err := json.Unmarshal([]byte(mustJSON(t, r)), &stored); err != nil {
				t.Fatal(err)
			}
			r = &stored
			x = r.Review
			host := hostResult(r, true)
			if done.Status != "cancelled" || x.Reason != c.reason || host.Reason != c.reason || r.Attempts[0].Reason != c.reason {
				t.Fatalf("review=%+v host=%+v attempt=%+v", x, host, r.Attempts[0])
			}
			if x.ReportedModel != "reported" || x.Usage.Tokens != 120 || host.Usage != x.Usage || r.Attempts[0].Usage != x.Usage {
				t.Fatal("stopped review lost reported usage")
			}
			if c.reason == "timeout" && !strings.Contains(x.Diagnostic, "10m0s") {
				t.Fatalf("missing executor limit: %s", x.Diagnostic)
			}
		})
	}
}

func TestTimeoutPipelineMetadata(t *testing.T) {
	want := TokenUsage{Tokens: 120, InputTokens: 100, OutputTokens: 20}
	r := &Run{Status: "running", Review: &Execution{ID: "review"}, Pipeline: &Pipeline{Stages: []StageResult{{Name: "review"}}, Active: &Step{ID: "review", Kind: "review", Stage: "review"}}}
	answer := agentAnswer{Reason: "timeout", Diagnostic: "executor limit 10m0s exceeded", Model: "reported", Usage: want}
	if err := completeStep(r, stepOutcome{Step: "review", Status: "cancelled", Answer: answer}); err != nil {
		t.Fatal(err)
	}
	if r.Review.Reason != "timeout" || r.Review.Diagnostic != answer.Diagnostic || r.Review.ReportedModel != "reported" || r.Review.Usage != want || len(r.Attempts) != 1 || r.Attempts[0].Reason != "timeout" || r.Attempts[0].Usage != want {
		t.Fatalf("metadata lost: review=%+v attempts=%+v", r.Review, r.Attempts)
	}
}

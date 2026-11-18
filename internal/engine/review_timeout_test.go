package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func TestTimeoutReviewMetadata(t *testing.T) {
	expired, stop := context.WithDeadline(runner.WithProcessTimeout(context.Background(), 7*time.Minute), time.Unix(0, 0))
	defer stop()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []struct {
		reason string
		ctx    context.Context
	}{{"timeout", expired}, {"cancelled", cancelled}} {
		t.Run(c.reason, func(t *testing.T) {
			x := &Execution{ID: "review", Config: ReviewerConfig{Backend: "pi"}}
			stdout := []byte(`{"type":"message_end","message":{"role":"assistant","model":"reported","usage":{"input":100,"output":20,"totalTokens":120,"cost":{"total":0.5}}}}`)
			done := reviewFrom(x, -1, outputBuffer(stdout, 65536), outputBuffer(nil, 65536), nil, executorStop(c.ctx))
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
			if c.reason == "timeout" && !strings.Contains(x.Diagnostic, "7m0s") {
				t.Fatalf("missing executor limit: %s", x.Diagnostic)
			}
		})
	}
}

func TestTimeoutPipelineMetadata(t *testing.T) {
	want := TokenUsage{Tokens: 120, InputTokens: 100, OutputTokens: 20}
	r := &Run{Status: "running", Review: &Execution{ID: "review"}, Pipeline: &Pipeline{Stages: []StageResult{{Name: "review"}}, Active: &Step{ID: "review", Kind: "review", Stage: "review"}}}
	answer := agentAnswer{Reason: "timeout", Diagnostic: "review executor limit 45m0s exceeded: context deadline exceeded", Model: "reported", Usage: want}
	if err := completeStep(r, stepOutcome{Step: "review", Status: "cancelled", Answer: answer}); err != nil {
		t.Fatal(err)
	}
	if r.Review.Reason != "timeout" || r.Review.Diagnostic != answer.Diagnostic || r.Review.ReportedModel != "reported" || r.Review.Usage != want || len(r.Attempts) != 1 || r.Attempts[0].Reason != "timeout" || r.Attempts[0].Usage != want {
		t.Fatalf("metadata lost: review=%+v attempts=%+v", r.Review, r.Attempts)
	}
}

func TestAdoptStoppedPipelineReview(t *testing.T) {
	for _, stopReason := range []string{"timeout", "cancelled", ""} {
		t.Run("stop_reason="+stopReason, func(t *testing.T) {
			r := &Run{
				Status:   "recovery-required",
				Configs:  []Snapshot{{Settings: Settings{ExecutorTimeout: "7m"}}},
				Host:     &Host{Name: "fixture", ID: "restart"},
				Review:   &Execution{ID: "review", InputDigest: "bound-input", Config: ReviewerConfig{Backend: "pi"}},
				Pipeline: &Pipeline{Stages: []StageResult{{Name: "review"}}},
				Recovery: &Recovery{Kind: "review", Stage: "review", Execution: "review"},
			}
			observed := []runner.ObservedExecutor{{
				ExecutorEvidence: runner.ExecutorEvidence{ID: "review.0", State: "stopped", Exit: -1},
				Record:           runner.ExecutorRecord{InputDigest: "bound-input", Stopped: true, StopReason: stopReason, Reaped: true},
			}}
			if !adoptRecorded(r, observed) {
				t.Fatal("reaped stopped review was not adopted")
			}
			want := stopReason
			if want == "" {
				want = "cancelled"
			}
			if r.Review.Status != "cancelled" || r.Review.Reason != want || hostResult(r, true).Reason != want || len(r.Attempts) != 1 || r.Attempts[0].Reason != want || r.Recovery != nil {
				t.Fatalf("adoption lost stop reason: review=%s reason=%q attempts=%+v recovery=%+v", r.Review.Status, r.Review.Reason, r.Attempts, r.Recovery)
			}
			if want == "timeout" && !strings.Contains(r.Review.Diagnostic, "7m0s") {
				t.Fatalf("timeout lost configured limit: %s", r.Review.Diagnostic)
			}
		})
	}
}

func TestAdoptFinalizedTimeoutValidatesInputAndUsage(t *testing.T) {
	for _, valid := range []bool{true, false} {
		x := &Execution{ID: "review", Input: []byte(`{"Head":"candidate","Paths":["sample"]}`), InputDigest: "bound-input", Config: ReviewerConfig{Backend: "pi"}}
		r := &Run{Status: "recovery-required", Configs: []Snapshot{{Settings: Settings{ExecutorTimeout: "1s"}}}, Review: x, Pipeline: &Pipeline{Stages: []StageResult{{Name: "review"}}}, Recovery: &Recovery{Kind: "review", Stage: "review", Execution: "review"}}
		finalInput := runner.BytesDigest(reviewFinalizationInput(x, reviewOutcome{}, true))
		if !valid {
			finalInput = "unrelated-input"
		}
		var observed []runner.ObservedExecutor
		for i, digest := range []string{x.InputDigest, finalInput} {
			observed = append(observed, runner.ObservedExecutor{
				ExecutorEvidence: runner.ExecutorEvidence{ID: fmt.Sprintf("review.%d", i), State: "stopped", Exit: -1},
				Record:           runner.ExecutorRecord{InputDigest: digest, Stopped: true, StopReason: "timeout", Reaped: true},
				Stdout:           []byte(`{"type":"message_end","message":{"role":"assistant","usage":{"totalTokens":120}}}`),
			})
		}
		observed[0].Stdout = []byte(`{"type":"message_end","message":{"role":"assistant","model":"reported","usage":{"totalTokens":120}}}`)
		if adopted := adoptRecorded(r, observed); adopted != valid {
			t.Fatalf("adopted=%t for valid input=%t", adopted, valid)
		}
		if valid && (!x.Finalization || x.ReportedModel != "reported" || x.Reason != "timeout" || x.Usage.Tokens != 240 || len(r.Attempts) != 1 || r.Attempts[0].Usage.Tokens != 240) {
			t.Fatalf("finalized timeout lost evidence: finalization=%t reason=%s usage=%+v attempts=%+v", x.Finalization, x.Reason, x.Usage, r.Attempts)
		}
	}
}

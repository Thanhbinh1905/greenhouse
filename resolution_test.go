package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resolutionPipeline(t *testing.T, mode string) (string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home, v := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "no documentation", "push": "local fixture", "pr": "local fixture", "ci": "local fixture"}}
	})
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	if mode != "auto" {
		v = mutate(t, bin, repo, home, v, "apply", "mode-"+mode, map[string]any{"mode": mode, "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
	}
	return bin, repo, home, v
}

func capturedReviewInput(t *testing.T, root string) map[string]json.RawMessage {
	t.Helper()
	var input map[string]json.RawMessage
	b, _ := os.ReadFile(filepath.Join(root, "review-input.json"))
	json.Unmarshal(b, &input)
	return input
}

func TestFixedResponseIsCheckedByTheNextIndependentReview(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := resolutionPipeline(t, "auto")
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("clean"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	p := run["pipeline"].(map[string]any)
	if run["status"] != "locally-ready" || p["outcome"] != "repaired-pass" {
		t.Fatalf("fixed and confirmed finding must reach a repaired pass: %v", p)
	}
	repair := p["repairs"].([]any)[0].(map[string]any)
	responses := repair["responses"].([]any)
	finding := run["findings"].([]any)[0].(map[string]any)
	if len(responses) != 1 || responses[0].(map[string]any)["finding"] != finding["id"] || responses[0].(map[string]any)["response"] != "fixed" {
		t.Fatalf("repair responses = %v", repair["responses"])
	}
	input := capturedReviewInput(t, root)
	if !strings.Contains(string(input["Responses"]), `"response":"fixed"`) || !strings.Contains(string(input["Contract"]), "resolutions") {
		t.Fatalf("rereview did not receive the responses: %s / %s", input["Responses"], input["Contract"])
	}
	checks := run["review"].(map[string]any)["result"].(map[string]any)["resolutions"].([]any)
	if len(checks) != 1 || checks[0].(map[string]any)["check"] != "confirmed" {
		t.Fatalf("review checks = %v", checks)
	}
	resolutions := p["resolutions"].([]any)
	if len(resolutions) != 1 || resolutions[0].(map[string]any)["kind"] != "fixed" {
		t.Fatalf("resolutions = %v", resolutions)
	}
	screen := terminalOutput(t, bin, repo, home, "q\n")
	for _, want := range []string{"Response fixed", "Review check confirmed", "Resolved (fixed)"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("terminal lacks %q:\n%s", want, screen)
		}
	}
}

func TestDisputeIsNeverCountedAsAFix(t *testing.T) {
	t.Parallel()
	for _, check := range []string{"confirmed", "rejected"} {
		t.Run(check, func(t *testing.T) {
			bin, repo, home, v := resolutionPipeline(t, "auto")
			root := filepath.Dir(home)
			os.WriteFile(filepath.Join(root, "fix-response"), []byte("disputed"), 0600)
			os.WriteFile(filepath.Join(root, "review-check"), []byte(check), 0600)
			os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("clean"), 0600)
			if check == "rejected" {
				os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("findings"), 0600)
			}
			v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
			done := waitPipeline(t, bin, repo, home)
			run := runOf(t, done)
			p := run["pipeline"].(map[string]any)
			repair := p["repairs"].([]any)[0].(map[string]any)
			if repair["status"] != "responded-no-change" || repair["head"] != "" || repair["responses"].([]any)[0].(map[string]any)["response"] != "disputed" {
				t.Fatalf("a dispute without edits must not commit: %v", repair)
			}
			if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] < 2 {
				t.Fatalf("the dispute was not independently rereviewed: %v", got)
			}
			if !strings.Contains(string(capturedReviewInput(t, root)["Responses"]), `"response":"disputed"`) {
				t.Fatal("rereview did not receive the dispute")
			}
			if p["outcome"] == "repaired-pass" {
				t.Fatalf("a dispute was counted as a fix: %v", p)
			}
			resolutions, _ := p["resolutions"].([]any)
			for _, r := range resolutions {
				if r.(map[string]any)["kind"] == "fixed" {
					t.Fatalf("a dispute was recorded as fixed: %v", r)
				}
			}
			if check == "confirmed" {
				if run["status"] != "locally-ready" || p["outcome"] != "clean-pass" || p["resolutions"].([]any)[0].(map[string]any)["kind"] != "dispute-upheld" {
					t.Fatalf("an upheld dispute must withdraw the finding: %v", p)
				}
				screen := terminalOutput(t, bin, repo, home, "q\n")
				if !strings.Contains(screen, "Response disputed") || !strings.Contains(screen, "Resolved (dispute upheld)") {
					t.Fatalf("terminal must show the dispute:\n%s", screen)
				}
				return
			}
			if run["candidate"] != strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")) {
				t.Fatalf("rejected dispute changed the candidate")
			}
			actionable := 0
			for _, f := range run["findings"].([]any) {
				if !strings.Contains(jsonText(p["resolutions"]), f.(map[string]any)["id"].(string)) {
					actionable++
				}
			}
			if actionable == 0 || run["status"] == "locally-ready" {
				t.Fatalf("a rejected dispute must leave the finding outstanding: %v", run)
			}
		})
	}
}

func TestInvalidResolutionResponsesRefuseTheRepair(t *testing.T) {
	t.Parallel()
	for name, response := range map[string]string{"missing": "missing", "deferred-without-authority": "deferred"} {
		t.Run(name, func(t *testing.T) {
			bin, repo, home, v := resolutionPipeline(t, "auto")
			root := filepath.Dir(home)
			os.WriteFile(filepath.Join(root, "fix-response"), []byte(response), 0600)
			v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
			done := waitPipeline(t, bin, repo, home)
			repair := runOf(t, done)["pipeline"].(map[string]any)["repairs"].([]any)[0].(map[string]any)
			if repair["status"] != "fixer-failed" || repair["head"] != "" || !strings.Contains(repair["log"].(string), "resolution response") {
				t.Fatalf("invalid responses must refuse the repair: %v", repair)
			}
		})
	}
}

func TestDeferralCitesAnsweredDecision(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := resolutionPipeline(t, "ask")
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "fix-response"), []byte("deferred"), 0600)
	os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("clean"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	v = waitPipeline(t, bin, repo, home)
	gate, _ := v["gate_id"].(string)
	if v = decide(t, bin, repo, home, v, "repair-ask", "repair", nil); v["outcome"] != "ok" {
		t.Fatal(v)
	}
	done := waitPipeline(t, bin, repo, home)
	p := runOf(t, done)["pipeline"].(map[string]any)
	response := p["repairs"].([]any)[0].(map[string]any)["responses"].([]any)[0].(map[string]any)
	if response["response"] != "deferred" || response["authority"] != gate {
		t.Fatalf("deferral must cite the answered decision: %v", response)
	}
	if p["outcome"] != "accepted-risk" || p["resolutions"].([]any)[0].(map[string]any)["kind"] != "deferred" {
		t.Fatalf("a confirmed deferral is accepted risk, not a fix: %v", p)
	}
	screen := terminalOutput(t, bin, repo, home, "q\n")
	if !strings.Contains(screen, "Response deferred") || !strings.Contains(screen, "authority "+short(gate)) {
		t.Fatalf("terminal must show the deferral and its authority:\n%s", screen)
	}
}

func TestOnePassRepairShowsResponses(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := repairFixture(t)
	v = repairRequest(t, bin, repo, home, v, "repair", "repair", selectedID(t, v))
	v = waitRepair(t, bin, repo, home)
	repair := runOf(t, v)["repair"].(map[string]any)
	responses, _ := repair["responses"].([]any)
	if repair["status"] != "fixed-not-independently-rereviewed" || len(responses) != 1 || responses[0].(map[string]any)["response"] != "fixed" {
		t.Fatalf("one-pass repair responses = %v", repair)
	}
	screen := terminalOutput(t, bin, repo, home, "q\n")
	if !strings.Contains(screen, "Response fixed") || !strings.Contains(screen, "Not independently checked") {
		t.Fatalf("terminal must show one-pass responses as unchecked:\n%s", screen)
	}
}

// respondFixed is the fake Pi fixer answer that reports every selected
// finding as fixed.
const respondFixed = `
import json as _json, sys as _sys
try:
    b
except NameError:
    b=_json.load(_sys.stdin)
print(_json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':_json.dumps({'responses':[{'finding':f['id'],'response':'fixed','reason':'edited the selected lines'} for f in b['Selected']]})}]}}))
print(_json.dumps({'type':'agent_settled','aborted':False}))
`

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostFailedReviewDiagnostics(t *testing.T) {
	for _, c := range []struct{ name, mode, reason, message string }{
		{"crash", "import sys; sys.stderr.write('backend crashed: token=fake-private-value\\n'); sys.exit(1)", "crash", "backend crashed"},
		{"zero-exit-model", `print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'error','errorMessage':'404 model not found apiKey=fake-private-value','content':[]}})); sys.exit(0)`, "model_mismatch", "404 model not found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			bin, repo, home := fixture(t)
			target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
			os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
			git(t, repo, "add", "sample.txt")
			git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
			installFake(t, home, c.mode)
			b, _ := json.Marshal(map[string]any{"version": 2, "op": "attach", "host": fakeHost})
			v := axi(t, bin, repo, home, string(b))
			v = hostPrepare(t, bin, repo, home, v, target, "-diagnostic")
			v = hostCall(t, bin, repo, home, v, "review", "failed-review", map[string]any{"grant": "failed-round"})
			if v["outcome"] != "ok" {
				t.Fatalf("review refused: %v", v)
			}
			v = hostSettle(t, bin, repo, home)
			result := v["host_result"].(map[string]any)
			diagnostic, _ := result["diagnostic"].(string)
			if result["reason"] != c.reason || !strings.Contains(diagnostic, c.message) {
				t.Fatalf("host cannot diagnose backend failure: %v", result)
			}
			if strings.Contains(diagnostic, "fake-private-value") {
				t.Fatalf("credential leaked: %v", result)
			}
		})
	}
}

func TestHostModelFallback(t *testing.T) {
	for _, configured := range []string{"", "explicit-model"} {
		r := &Run{Host: &Host{Name: "host", ID: "1"}, Review: &Execution{Config: ReviewerConfig{Model: configured}, Result: &Verdict{ReportedModel: "reported-model"}}}
		want := configured
		if want == "" {
			want = "reported-model"
		}
		if got := hostResult(r, true).Model; got != want {
			t.Fatalf("model = %q, want %q", got, want)
		}
	}
}

func TestHostReviewMetadataSurvivesPersistence(t *testing.T) {
	r := &Run{Host: &Host{Name: "host", ID: "1"}, Review: &Execution{ID: "review-1"}}
	want := TokenUsage{InputTokens: 100, OutputTokens: 20, CachedInputTokens: 80, Tokens: 120, CostUSD: 0.5}
	retainReviewMetadata(r.Review, reviewOutcome{Status: "inconclusive", Answer: agentAnswer{Reason: "model_mismatch", Diagnostic: "model not found", Model: "reported-model", Usage: want}})
	recordAttempt(r, Attempt{Execution: "review-1", Kind: "review", Tokens: want.Tokens, CostUSD: want.CostUSD})
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var restored Run
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	result := hostResult(&restored, true)
	if result.Reason != "model_mismatch" || result.Diagnostic != "model not found" || result.Model != "reported-model" || result.Usage != want || restored.Attempts[0].Tokens != 120 || restored.Attempts[0].Usage != want {
		t.Fatalf("metadata lost: %+v attempts %+v", result, restored.Attempts)
	}
}

func TestHostCodexDefaultModelUsage(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	installFake(t, home, "")
	installAgentCLIs(t, home)
	b, _ := json.Marshal(map[string]any{"version": 2, "op": "attach", "host": fakeHost})
	v := axi(t, bin, repo, home, string(b))
	v = hostPrepare(t, bin, repo, home, v, target, "-default-model")
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 1, "rules": []string{"unslop"}, "reviewer_backend": "codex", "reviewer_model": "", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	v = hostCall(t, bin, repo, home, v, "apply", "default-model", map[string]any{"settings": settings})
	v = hostCall(t, bin, repo, home, v, "resolve-rules", "codex-rules", nil)
	v = hostCall(t, bin, repo, home, v, "review", "codex-review", map[string]any{"grant": "codex-round"})
	if v["outcome"] != "ok" {
		t.Fatalf("review refused: %v", v)
	}
	v = hostSettle(t, bin, repo, home)
	result := v["host_result"].(map[string]any)
	usage, _ := result["usage"].(map[string]any)
	if result["status"] != "clean-review" || result["model"] != "gpt-5.6-luna" || usage["input_tokens"] != float64(13395) || usage["output_tokens"] != float64(15) || usage["cached_input_tokens"] != float64(0) || usage["tokens"] != float64(13410) {
		t.Fatalf("host metadata: %v", result)
	}
	attempts := runOf(t, v)["attempts"].([]any)
	if attempts[len(attempts)-1].(map[string]any)["tokens"] != usage["tokens"] {
		t.Fatalf("attempt total differs: %v", attempts)
	}
}

func TestHostRecoveredStructuredError(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	installFake(t, home, `print(json.dumps({'type':'message_end','message':{'role':'assistant','stopReason':'error','errorMessage':'404 model not found','content':[]}}))`)
	b, _ := json.Marshal(map[string]any{"version": 2, "op": "attach", "host": fakeHost})
	v := axi(t, bin, repo, home, string(b))
	v = hostPrepare(t, bin, repo, home, v, target, "-recovered-error")
	v = hostCall(t, bin, repo, home, v, "review", "recovered-review", map[string]any{"grant": "recovered-round"})
	if v["outcome"] != "ok" {
		t.Fatalf("review refused: %v", v)
	}
	result := hostSettle(t, bin, repo, home)["host_result"].(map[string]any)
	if result["verdict"] != "findings" || len(result["findings"].([]any)) != 1 || result["reason"] != nil || result["diagnostic"] != nil {
		t.Fatalf("recovered final findings discarded: %v", result)
	}
}

func TestHostRecoveredCodexError(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	installFake(t, home, "")
	root := installAgentCLIs(t, home)
	path := filepath.Join(root, "fake-codex")
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, path, strings.Replace(string(script), "text=verdict()", "print(json.dumps({'type':'error','message':'stream disconnected before completion; reconnecting 1/5'}))\ntext=verdict()", 1))
	b, _ := json.Marshal(map[string]any{"version": 2, "op": "attach", "host": fakeHost})
	v := axi(t, bin, repo, home, string(b))
	v = hostPrepare(t, bin, repo, home, v, target, "-codex-recovery")
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 1, "rules": []string{"unslop"}, "reviewer_backend": "codex", "reviewer_model": "", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	v = hostCall(t, bin, repo, home, v, "apply", "codex-recovery", map[string]any{"settings": settings})
	v = hostCall(t, bin, repo, home, v, "resolve-rules", "codex-recovery-rules", nil)
	v = hostCall(t, bin, repo, home, v, "review", "codex-recovery-review", map[string]any{"grant": "codex-recovery-round"})
	if v["outcome"] != "ok" {
		t.Fatalf("review refused: %v", v)
	}
	result := hostSettle(t, bin, repo, home)["host_result"].(map[string]any)
	if result["status"] != "clean-review" || result["verdict"] != "clean" || result["model"] != "gpt-5.6-luna" || result["reason"] != nil || result["diagnostic"] != nil {
		t.Fatalf("recovered clean verdict discarded: %v", result)
	}
}

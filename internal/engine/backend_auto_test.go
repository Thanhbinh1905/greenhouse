package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeBackends(t *testing.T, home string, extra map[string]any) {
	t.Helper()
	root := filepath.Dir(home)
	config := map[string]any{
		"claude": map[string]any{"executable": filepath.Join(root, "fake-claude")},
		"codex":  map[string]any{"executable": filepath.Join(root, "fake-codex"), "provider": map[string]any{"base_url": "http://127.0.0.1:1/v1"}},
	}
	for k, v := range extra {
		config[k] = v
	}
	writePrivateJSON(t, filepath.Join(home, "backends.json"), config)
}

func eventsOf(t *testing.T, v map[string]any, kind string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range runOf(t, v)["events"].([]any) {
		if event := e.(map[string]any); event["kind"] == kind {
			out = append(out, event)
		}
	}
	return out
}

func waitStatus(t *testing.T, bin, repo, home string, want ...string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		v := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		if has(want, runOf(t, v)["status"].(string)) {
			return v
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run never reached %v: %v", want, axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)["explanation"])
	return nil
}

func TestAutoResumeWithBackoffSurvivesDaemonRestart(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := onePassWith(t, "codex", "gpt-5.6-luna")
	writeBackends(t, home, map[string]any{"auto_resume": map[string]any{"max_attempts": 2, "base_seconds": 2, "max_seconds": 2}})
	setModes(t, root, "codex", "rate-limit", "success")
	mutate(t, bin, repo, home, v, "review", "review-rate-limit", nil)
	v = waitStatus(t, bin, repo, home, "paused")
	pause := pauseOf(t, v)
	if pause["reset_at"] != nil || pause["auto_resume_at"] == nil || !strings.Contains(v["explanation"].(string), "Auto mode will resume at") || !has(strs(v["actions"]), "cancel-auto-resume") {
		t.Fatalf("unknown reset must schedule a bounded retry: %v %v", pause, v["explanation"])
	}
	stopDaemon(t, home)
	time.Sleep(2500 * time.Millisecond)
	v = waitStatus(t, bin, repo, home, "clean-review")
	resumes := eventsOf(t, v, "resume")
	if len(resumes) != 1 || resumes[0]["actor"] != "auto" || runOf(t, v)["review_used"].(float64) != 1 {
		t.Fatalf("restart must keep the scheduled automatic resume: %v", runOf(t, v)["events"])
	}
}

func TestAutoResumeStopsAfterRetryLimit(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := onePassWith(t, "codex", "gpt-5.6-luna")
	writeBackends(t, home, map[string]any{"auto_resume": map[string]any{"max_attempts": 1, "base_seconds": 1, "max_seconds": 1}})
	setModes(t, root, "codex", "rate-limit")
	mutate(t, bin, repo, home, v, "review", "review-exhaust", nil)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		if runOf(t, v)["status"] == "paused" && pauseOf(t, v)["auto_resume_at"] == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(v["explanation"].(string), "Automatic retries are exhausted (1 of 1)") || len(agentCalls(t, root, "codex")) != 2 || runOf(t, v)["review_used"].(float64) != 0 {
		t.Fatalf("one automatic retry, then stay paused without budget loss: %v calls=%d", v["explanation"], len(agentCalls(t, root, "codex")))
	}
}

func TestAskModeNeverResumesAutomatically(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := onePassWith(t, "codex", "gpt-5.6-luna")
	writeBackends(t, home, map[string]any{"auto_resume": map[string]any{"max_attempts": 3, "base_seconds": 1, "max_seconds": 1}, "fallback": map[string]any{"reviewer": []map[string]any{{"backend": "claude", "model": "claude-haiku-5-5"}}}})
	v = mutate(t, bin, repo, home, v, "apply", "ask", map[string]any{"mode": "ask", "preset": "one-pass", "limit": 4, "rules": []string{"unslop", "no-comments"}, "reviewer_backend": "codex", "reviewer_model": "gpt-5.6-luna", "fixer_backend": "pi", "fixer_model": "fixture/model", "auto_resume": "on"})
	v = mutate(t, bin, repo, home, v, "resolve-rules", "rules-ask", nil)
	setModes(t, root, "codex", "quota", "success")
	mutate(t, bin, repo, home, v, "review", "review-ask", nil)
	waitStatus(t, bin, repo, home, "paused")
	time.Sleep(2 * time.Second)
	v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	pause := pauseOf(t, v)
	if runOf(t, v)["status"] != "paused" || pause["auto_resume_at"] != nil || len(agentCalls(t, root, "claude")) != 0 || !strings.Contains(v["explanation"].(string), "Ask mode never resumes automatically; fallback claude claude-haiku-5-5 is configured") {
		t.Fatalf("ask mode must park for the User: %v %v", pause, v["explanation"])
	}
}

func TestAutoModePrefersConfiguredFallbackOverWaiting(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := onePassWith(t, "codex", "gpt-5.6-luna")
	writeBackends(t, home, map[string]any{"fallback": map[string]any{"reviewer": []map[string]any{{"backend": "claude", "model": "claude-haiku-5-5"}}}})
	setModes(t, root, "codex", "quota")
	mutate(t, bin, repo, home, v, "review", "review-fallback", nil)
	v = waitStatus(t, bin, repo, home, "clean-review", "paused-debug")
	run := runOf(t, v)
	var switches []map[string]any
	for _, e := range eventsOf(t, v, "backend-switch") {
		if e["from"] == "codex gpt-5.6-luna" {
			switches = append(switches, e)
		}
	}
	settings := run["configs"].([]any)[len(run["configs"].([]any))-1].(map[string]any)["settings"].(map[string]any)
	result := run["review"].(map[string]any)["result"].(map[string]any)
	if len(switches) != 1 || switches[0]["actor"] != "auto" || switches[0]["from"] != "codex gpt-5.6-luna" || switches[0]["to"] != "claude claude-haiku-5-5" || settings["reviewer_backend"] != "claude" || result["reported_provider"] != "claude" {
		t.Fatalf("quota must switch to the configured fallback without waiting: %v", run["events"])
	}
	attempts := run["attempts"].([]any)
	if len(attempts) != 2 || attempts[0].(map[string]any)["backend"] != "codex" || attempts[1].(map[string]any)["backend"] != "claude" {
		t.Fatalf("each backend attempt keeps its provenance: %v", attempts)
	}
}

func TestSwitchFromCodexToClaudeWhilePaused(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := pipelineWith(t, "codex", "gpt-5.6-luna", "pi", "fixture/model")
	setModes(t, root, "codex", "quota")
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	v = waitStatus(t, bin, repo, home, "paused")
	if pauseOf(t, v)["auto_resume_at"] == nil {
		t.Fatalf("auto mode must schedule the known reset: %v", pauseOf(t, v))
	}
	v = mutate(t, bin, repo, home, v, "apply", "switch-to-claude", map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "claude", "reviewer_model": "claude-haiku-5-5", "fixer_backend": "pi", "fixer_model": "fixture/model"})
	if v["outcome"] != "ok" || pauseOf(t, v)["auto_resume_at"] != nil || !strings.Contains(pauseOf(t, v)["auto_note"].(string), "cancelled by axi") {
		t.Fatalf("a settings change must cancel the pending automatic resume: %v", v)
	}
	var switches []map[string]any
	for _, e := range eventsOf(t, v, "backend-switch") {
		if e["from"] == "codex gpt-5.6-luna" {
			switches = append(switches, e)
		}
	}
	if len(switches) != 1 || switches[0]["actor"] != "axi" || switches[0]["from"] != "codex gpt-5.6-luna" || switches[0]["to"] != "claude claude-haiku-5-5" || switches[0]["reason"] != "reviewer: test adjustment" {
		t.Fatalf("switch must record old and new backend, model, actor and reason: %v", switches)
	}
	mutate(t, bin, repo, home, v, "resume", "resume-after-switch", nil)
	v = waitStatus(t, bin, repo, home, "locally-ready")
	run := runOf(t, v)
	review := run["review"].(map[string]any)
	if review["backend"] != "claude-stream-json-v1" || review["result"].(map[string]any)["reported_model"] != "claude-haiku-5-5" {
		t.Fatalf("resumed review must run on the new backend: %v", review)
	}
	reviews := run["pipeline"].(map[string]any)["reviews"].([]any)
	if len(reviews) != 1 || reviews[0].(map[string]any)["backend"] != "codex-json-v1" || reviews[0].(map[string]any)["status"] != "paused" {
		t.Fatalf("the paused codex attempt keeps its provenance: %v", reviews)
	}
}

func TestPausedOnePassReviewSurvivesDaemonRestart(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := onePassWith(t, "codex", "gpt-5.6-luna")
	writeBackends(t, home, map[string]any{"auto_resume": map[string]any{"max_attempts": 3, "base_seconds": 3600, "max_seconds": 3600}})
	setModes(t, root, "codex", "rate-limit", "success")
	mutate(t, bin, repo, home, v, "review", "review-before-restart", nil)
	v = waitStatus(t, bin, repo, home, "paused")
	scheduled := pauseOf(t, v)["auto_resume_at"]
	stopDaemon(t, home)
	v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	if v["outcome"] != "ok" || runOf(t, v)["status"] != "paused" || runOf(t, v)["review_used"].(float64) != 0 || pauseOf(t, v)["auto_resume_at"] != scheduled {
		t.Fatalf("restart must keep the paused run, its budget and its schedule: %v", v)
	}
}

func TestQuotaResetTimeDecidesAutomaticResume(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ backend, model string }{{"claude", "claude-haiku-5-5"}, {"codex", "gpt-5.6-luna"}} {
		t.Run(c.backend, func(t *testing.T) {
			t.Parallel()
			for _, reset := range []string{"3600", "-120"} {
				bin, repo, home, root, v := onePassWith(t, c.backend, c.model)
				os.WriteFile(filepath.Join(root, "reset-in-"+c.backend), []byte(reset), 0600)
				setModes(t, root, c.backend, "quota", "success")
				mutate(t, bin, repo, home, v, "review", "review-reset-"+reset, nil)
				if reset == "3600" {
					waitStatus(t, bin, repo, home, "paused")
					time.Sleep(time.Second)
					v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
					at, err := time.Parse(time.RFC3339, pauseOf(t, v)["auto_resume_at"].(string))
					if runOf(t, v)["status"] != "paused" || err != nil || time.Until(at) < 50*time.Minute {
						t.Fatalf("a future reset must keep the run paused until then: %v", pauseOf(t, v))
					}
					continue
				}
				v = waitStatus(t, bin, repo, home, "clean-review")
				if resumes := eventsOf(t, v, "resume"); len(resumes) != 1 || resumes[0]["actor"] != "auto" {
					t.Fatalf("a passed reset must resume automatically: %v", runOf(t, v)["events"])
				}
			}
		})
	}
}

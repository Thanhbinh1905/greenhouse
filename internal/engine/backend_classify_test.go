package engine

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func recorded(t *testing.T, backend, name string) ([]byte, []byte, int) {
	t.Helper()
	base := filepath.Join("..", "..", "testdata", "backends", backend, name)
	stdout, err := os.ReadFile(base + ".stdout")
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(base + ".stderr")
	if err != nil {
		t.Fatal(err)
	}
	exit, err := os.ReadFile(base + ".exit")
	if err != nil {
		t.Fatal(err)
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(exit)))
	if err != nil {
		t.Fatal(err)
	}
	return stdout, stderr, code
}

func TestClassifyRecordedBackendOutputs(t *testing.T) {
	t.Parallel()
	bangkok := time.FixedZone("+07", 7*3600)
	recordedAt := time.Date(2026, 11, 8, 20, 47, 17, 0, bangkok)
	for _, c := range []struct {
		backend, name, reason, reset string
	}{
		{"claude", "success", "", ""},
		{"claude", "auth", "auth", ""},
		{"claude", "quota", "quota", "2026-11-08T14:50:46Z"},
		{"claude", "api-rate-limit", "quota", ""},
		{"claude", "unavailable", "unavailable", ""},
		{"claude", "connection-refused", "unavailable", ""},
		{"codex", "success", "", ""},
		{"codex", "auth", "auth", ""},
		{"codex", "quota", "quota", "2026-11-08T14:47:00Z"},
		{"codex", "quota-date", "quota", "2026-11-11T13:47:00Z"},
		{"codex", "insufficient-quota", "quota", ""},
		{"codex", "rate-limit", "quota", ""},
		{"codex", "unavailable", "unavailable", ""},
		{"codex", "waiting-for-network", "unavailable", ""},
		{"pi", "auth", "auth", ""},
		{"pi", "quota", "quota", ""},
		{"pi", "rate-limit", "quota", ""},
		{"pi", "unavailable", "unavailable", ""},
		{"pi", "connection-refused", "unavailable", ""},
	} {
		stdout, stderr, code := recorded(t, c.backend, c.name)
		failure := classifyBackend(c.backend, code, stdout, stderr, recordedAt)
		if c.reason == "" {
			if failure != nil {
				t.Errorf("%s/%s: unexpected failure %+v", c.backend, c.name, failure)
			}
			continue
		}
		if failure == nil || failure.Reason != c.reason || failure.ResetAt != c.reset {
			t.Errorf("%s/%s: got %+v, want reason %s reset %q", c.backend, c.name, failure, c.reason, c.reset)
			continue
		}
		if failure.Message == "" || strings.Contains(failure.Message, "sk-") || len(failure.Message) > 300 {
			t.Errorf("%s/%s: unsafe or empty message %q", c.backend, c.name, failure.Message)
		}
	}
}

func TestUsageIsReadFromFailedAndSuccessfulRuns(t *testing.T) {
	t.Parallel()
	stdout, stderr, _ := recorded(t, "claude", "success")
	if cost, tokens := agentUsage("claude", stdout, stderr); cost != 6.0519999999999997e-05 || tokens != 55 {
		t.Fatalf("claude success usage: %v %v", cost, tokens)
	}
	failed := strings.Replace(string(stdout), `"is_error":false`, `"is_error":true`, 1)
	if failed == string(stdout) {
		t.Fatal("fixture shape changed")
	}
	if cost, tokens := agentUsage("claude", []byte(failed), stderr); cost != 6.0519999999999997e-05 || tokens != 55 {
		t.Fatalf("claude error result must keep its usage: %v %v", cost, tokens)
	}
	stdout, stderr, _ = recorded(t, "codex", "success")
	if _, tokens := agentUsage("codex", stdout, stderr); tokens != 13410 {
		t.Fatalf("codex usage: %v", tokens)
	}
}

func TestDiagnosticRedactionAndBounds(t *testing.T) {
	secrets := []string{"bearer-private", "key-private", "dash-private", "token-private", "secret-private", "password-private", "sk-private123", "env-private", "quoted private value"}
	text := "Authorization: Bearer bearer-private\napi_key=key-private\napi-key: dash-private\ntoken=token-private\nsecret=secret-private\npassword=password-private\nsk-private123\nOPENAI_API_KEY=env-private\npassword=\"quoted private value\""
	got := diagnosticExcerpt(text, false)
	for _, secret := range secrets {
		if strings.Contains(got, secret) {
			t.Errorf("secret %q survived: %s", secret, got)
		}
	}
	for _, text := range []string{strings.Repeat("line\n", 80) + "last", strings.Repeat("x", 6000) + "last"} {
		got := diagnosticExcerpt(text, false)
		if len(got) > 4096 || !strings.HasPrefix(got, "[truncated]\n") || !strings.HasSuffix(got, "last") {
			t.Errorf("bad excerpt: length %d", len(got))
		}
	}
}

func TestBackendTokenBreakdown(t *testing.T) {
	for _, backend := range []string{"claude", "codex", "pi"} {
		t.Run(backend, func(t *testing.T) {
			var stdout []byte
			want := TokenUsage{}
			switch backend {
			case "claude":
				stdout, _, _ = recorded(t, backend, "success")
				want = TokenUsage{InputTokens: 2, OutputTokens: 53, CachedInputTokens: 3382, Tokens: 55, CostUSD: 6.0519999999999997e-05}
			case "codex":
				stdout = []byte(`{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":20,"cached_input_tokens":80}}`)
				want = TokenUsage{InputTokens: 100, OutputTokens: 20, CachedInputTokens: 80, Tokens: 120}
			case "pi":
				stdout = []byte(`{"type":"message_end","message":{"role":"assistant","usage":{"input":10,"output":5,"totalTokens":30,"cost":{"total":0.25}}}}`)
				want = TokenUsage{InputTokens: 10, OutputTokens: 5, Tokens: 30, CostUSD: 0.25}
			}
			if got := agentTokenUsage(backend, stdout, nil); got != want {
				t.Fatalf("usage = %+v, want %+v", got, want)
			}
		})
	}
}

func TestStderrOnlyBackendClassification(t *testing.T) {
	for _, c := range []struct{ text, reason string }{{"401 Unauthorized token=fake-secret", "auth"}, {"quota exhausted", "quota"}, {"model not found", "model_mismatch"}, {"panic: unexpected EOF", "crash"}, {"exec: codex: executable file not found", "missing_binary"}} {
		f := classifyBackend("codex", 1, nil, []byte(c.text), time.Now())
		if f == nil || f.Reason != c.reason {
			t.Errorf("%q: got %+v, want %s", c.text, f, c.reason)
		}
	}
}

func TestStructuredDiagnosticsExcludeReviewContent(t *testing.T) {
	for _, stdout := range []string{
		`{"type":"result","is_error":true,"result":"backend rejected password=private-value"}`,
		`{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"backend rejected token=private-value","content":[{"type":"text","text":"private review input"}]}}`,
	} {
		got := diagnosticExcerpt(backendDiagnostics([]byte(stdout), nil), false)
		if !strings.Contains(got, "backend rejected") || strings.Contains(got, "private-value") || strings.Contains(got, "private review input") {
			t.Fatalf("diagnostic = %q", got)
		}
	}
}

func TestUsageAggregatesTurnsAndClaudeCacheFallback(t *testing.T) {
	codex := []byte("{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2,\"cached_input_tokens\":8}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":20,\"output_tokens\":3,\"cached_input_tokens\":15}}")
	if got := agentTokenUsage("codex", codex, nil); got != (TokenUsage{InputTokens: 30, OutputTokens: 5, CachedInputTokens: 23, Tokens: 35}) {
		t.Fatalf("multiple turns: %+v", got)
	}
	claude := []byte(`{"type":"result","is_error":true,"total_cost_usd":0.25,"usage":{"input_tokens":2,"output_tokens":3,"cache_read_input_tokens":40}}`)
	if got := agentTokenUsage("claude", claude, nil); got != (TokenUsage{InputTokens: 2, OutputTokens: 3, CachedInputTokens: 40, Tokens: 5, CostUSD: 0.25}) {
		t.Fatalf("claude fallback: %+v", got)
	}
}

func TestDiagnosticRedactsEscapedQuotedValues(t *testing.T) {
	for _, text := range []string{`password="before\"private-value"`, `token='before\'private-value'`, `{"api_key":"before\"private-value"}`} {
		if got := diagnosticExcerpt(text, false); strings.Contains(got, "private-value") {
			t.Fatalf("quoted credential leaked: %s", got)
		}
	}
}

func TestDiagnosticRedactsIncompleteValues(t *testing.T) {
	for _, text := range []string{`password="private-value`, `token='private-value`, `secret=before\private-value`} {
		if got := diagnosticExcerpt(text, true); strings.Contains(got, "private-value") {
			t.Fatalf("incomplete credential leaked: %s", got)
		}
	}
}

func TestDiagnosticRedactsCamelCaseAPIKey(t *testing.T) {
	for _, text := range []string{`apiKey=camel-private-value`, `{"apiKey":"camel-private-value"}`, `apiKey: 'camel-private-value'`} {
		if got := diagnosticExcerpt(text, false); strings.Contains(got, "camel-private-value") {
			t.Fatalf("apiKey credential leaked: %s", got)
		}
	}
}

func TestClassificationUsesTerminalStructuredOutcome(t *testing.T) {
	for _, c := range []struct{ backend, failure string }{
		{"pi", `{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"404 model not found"}}`},
		{"codex", `{"type":"turn.failed","error":{"message":"model not found"}}`},
		{"claude", `{"type":"result","is_error":true,"api_error_status":401,"result":"authentication failed"}`},
	} {
		t.Run(c.backend, func(t *testing.T) {
			var success []byte
			if c.backend == "pi" {
				success = []byte(`{"type":"message_end","message":{"role":"assistant","stopReason":"stop"}}`)
			} else {
				success, _, _ = recorded(t, c.backend, "success")
			}
			recovered := append([]byte(c.failure+"\n"), success...)
			if got := classifyBackend(c.backend, 0, recovered, nil, time.Now()); got != nil {
				t.Fatalf("recovered failure classified: %+v", got)
			}
			terminal := append(append([]byte{}, success...), []byte("\n"+c.failure)...)
			if got := classifyBackend(c.backend, 0, terminal, nil, time.Now()); got == nil {
				t.Fatal("terminal structured failure ignored")
			}
		})
	}
}

func TestClassificationUsesLatestCodexFailure(t *testing.T) {
	stdout := []byte("{\"type\":\"turn.failed\",\"error\":{\"message\":\"model not found\"}}\n{\"type\":\"error\",\"message\":\"worker crashed\"}")
	failure := classifyBackend("codex", 0, stdout, nil, time.Now())
	if failure == nil || failure.Reason != "crash" {
		t.Fatalf("superseded error controls classification: %+v", failure)
	}
}

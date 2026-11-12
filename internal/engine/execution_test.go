package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReviewRequiresExplicitIntent(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	v = mutate(t, bin, repo, home, v, "review", "review-missing", nil)
	if v["outcome"] != "refused" || v["error"] == "unsupported operation: review" {
		t.Fatalf("review must validate prerequisites, got %v", v)
	}
}
func prepareFixture(t *testing.T) (string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	installFake(t, home, "")
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop", "no-comments"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	v = mutate(t, bin, repo, home, v, "apply", "fixture-model", settings)
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": "none", "action_id": "prepare", "intent": "Review sample change", "target": target}
	b, _ := json.Marshal(q)
	v = axi(t, bin, repo, home, string(b))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	v = mutate(t, bin, repo, home, v, "resolve-rules", "rules", nil)
	return bin, repo, home, v
}
func installFake(t *testing.T, home, mode string) string {
	t.Helper()
	allowFixtureWrites(t, home)
	providerPath := filepath.Join(home, "providers.json")
	if _, err := os.Stat(providerPath); os.IsNotExist(err) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "model"}, {"id": "gpt-5.6-luna"}, {"id": "gpt-6-luna"}}})
		}))
		t.Cleanup(server.Close)
		providerConfig := TrustedProviderConfig{Providers: map[string]ProviderConfig{"fixture": {BaseURL: server.URL, APIKeyEnv: "GREENHOUSE_TEST_PROVIDER_KEY", API: "openai-completions"}, "openai": {BaseURL: server.URL, APIKeyEnv: "GREENHOUSE_TEST_PROVIDER_KEY", API: "openai-completions"}}}
		providerBytes, _ := json.Marshal(providerConfig)
		if err := writeConfigBytes(providerPath, providerBytes, 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := filepath.Join(filepath.Dir(home), "fake-pi")
	script := `#!/usr/bin/python3
import sys,json,os,time
if '--list-models' in sys.argv:
 print('provider model context')
 print('fixture model 128K')
 print('openai gpt-5.6-luna 128K')
 print('openai gpt-6-luna 128K')
 sys.exit(0)
root=os.path.dirname(__file__)
with open(root+'/invocations','a') as f: f.write('invoked\n')
b=json.load(sys.stdin)
with open(root+'/captured.json','w') as f: json.dump({'args':sys.argv[1:],'input':b,'env':[k for k in os.environ if k.startswith(('PI_','POSSE_','HERDR_'))]},f)
MODE
v={'head':b['Head'],'verdict':'findings','coverage':b['Paths'],'findings':[{'id':'model-id','priority':'P2','origin':'policy','file':'sample.txt','start':1,'end':1,'explanation':'Changed prose is redundant','action':'Remove repetition','rule':'unslop/plain-prose','head':b['Head']}]}
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
`
	script = strings.Replace(script, "MODE", mode, 1)
	if e := os.WriteFile(p, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(ReviewerConfig{Executable: p, Model: "fixture/model"})
	if e := writeConfigBytes(filepath.Join(home, "reviewer.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	return filepath.Join(filepath.Dir(home), "invocations")
}
func waitReview(t *testing.T, bin, repo, home string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		v := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		status := runOf(t, v)["status"]
		if status != "running" && status != "cancelling" {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("review did not settle")
	return nil
}
func TestExecutableReviewPersistenceAndReplay(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	invocations := installFake(t, home, "")
	os.WriteFile(filepath.Join(repo, "dirty"), []byte("preserved"), 0600)
	before := git(t, repo, "status", "--porcelain")
	refs := git(t, repo, "show-ref")
	var a, b map[string]any
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a = mutate(t, bin, repo, home, v, "review", "review-1", nil) }()
	go func() { defer wg.Done(); b = mutate(t, bin, repo, home, v, "review", "review-1", nil) }()
	wg.Wait()
	if a["outcome"] != "ok" || b["outcome"] != "ok" {
		t.Fatal(a, b)
	}
	done := waitReview(t, bin, repo, home)
	if runOf(t, done)["status"] != "outstanding-findings" || runOf(t, done)["review_used"] != float64(1) {
		t.Fatal(done)
	}
	review := runOf(t, done)["review"].(map[string]any)
	if len(review["result"].(map[string]any)["findings"].([]any)) != 1 {
		t.Fatal(review)
	}
	captured, _ := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	if !strings.Contains(string(captured), "--no-context-files") || !strings.Contains(string(captured), "read,grep,find,ls") || !strings.Contains(string(captured), "plain-prose") || strings.Contains(string(captured), "POSSE_") {
		t.Fatal(string(captured))
	}
	var capturedInput struct {
		Input map[string]json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(captured, &capturedInput); err != nil {
		t.Fatal(err)
	}
	var delivered AgentInput
	if err := json.Unmarshal(capturedInput.Input["Criteria"], &delivered); err != nil {
		t.Fatal(err)
	}
	rules := review["rules"].(map[string]any)
	encoded, _ := json.Marshal(rules)
	var identity InputIdentity
	if err := json.Unmarshal(encoded, &identity); err != nil {
		t.Fatal(err)
	}
	var retained AgentInput
	if err := json.Unmarshal(identity.Content, &retained); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(delivered, retained) || identity.Digest != bytesDigest(identity.Content) {
		t.Fatal("emitted criteria differ from retained input identity")
	}
	if mutate(t, bin, repo, home, done, "review", "review-2", nil)["outcome"] != "refused" {
		t.Fatal("one-pass dispatched twice")
	}
	s := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 8, "rules": []string{"unslop", "no-comments"}}
	changed := mutate(t, bin, repo, home, done, "apply", "cap", s)
	if runOf(t, changed)["review_used"] != float64(1) {
		t.Fatal("usage reset")
	}
	stopDaemon(t, home)
	restarted := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	if runOf(t, restarted)["review"].(map[string]any)["input_digest"] != review["input_digest"] {
		t.Fatal("evidence lost")
	}
	count, _ := os.ReadFile(invocations)
	if string(count) != "invoked\n" {
		t.Fatal(string(count))
	}
	if git(t, repo, "status", "--porcelain") != before || git(t, repo, "show-ref") != refs {
		t.Fatal("source changed")
	}
}
func TestReviewCancellationAndMalformedOutput(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"time.sleep(30)", "print('malformed');sys.exit(0)"} {
		t.Run(mode, func(t *testing.T) {
			bin, repo, home, v := prepareFixture(t)
			installFake(t, home, mode)
			v = mutate(t, bin, repo, home, v, "review", "start", nil)
			if mode == "time.sleep(30)" {
				v = mutate(t, bin, repo, home, v, "cancel", "cancel", nil)
				if v["outcome"] != "ok" {
					t.Fatal(v)
				}
			}
			v = waitReview(t, bin, repo, home)
			want := "inconclusive"
			if mode == "time.sleep(30)" {
				want = "cancelled"
			}
			if runOf(t, v)["status"] != want || runOf(t, v)["review_used"] != float64(1) {
				t.Fatal(v)
			}
		})
	}
}
func TestReviewDeliversNumberedCandidateFiles(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	commit := func(msg string) string {
		git(t, repo, "add", "-A")
		git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", msg)
		return strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	}
	os.WriteFile(filepath.Join(repo, "old.txt"), []byte("gone\n"), 0600)
	os.WriteFile(filepath.Join(repo, "keep.txt"), []byte("a\nb\n"), 0600)
	target := commit("base")
	os.Remove(filepath.Join(repo, "old.txt"))
	os.WriteFile(filepath.Join(repo, "keep.txt"), []byte("a\nchanged\nc"), 0600)
	commit("candidate")
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	installFake(t, home, "")
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": "none", "action_id": "prepare", "intent": "Review files", "target": target}
	b, _ := json.Marshal(q)
	v = mutate(t, bin, repo, home, axi(t, bin, repo, home, string(b)), "resolve-rules", "rules", nil)
	mutate(t, bin, repo, home, v, "review", "review", nil)
	waitReview(t, bin, repo, home)
	captured, _ := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	var c struct {
		Input struct {
			Files    []map[string]string
			Contract string
		} `json:"input"`
	}
	if err := json.Unmarshal(captured, &c); err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{{"Path": "keep.txt", "Lines": "1: a\n2: changed\n3: c\n"}}
	if !reflect.DeepEqual(c.Input.Files, want) || !strings.Contains(c.Input.Contract, "Files") {
		t.Fatalf("delivered files = %v contract = %s", c.Input.Files, c.Input.Contract)
	}
}

func TestReviewFailureMetadata(t *testing.T) {
	for _, c := range []struct {
		name, backend, stdout, stderr, reason string
		code                                  int
	}{
		{name: "crash", backend: "codex", stderr: "panic: token=fake-value", reason: "crash", code: 1},
		{name: "auth", backend: "codex", stdout: `{"type":"turn.failed","error":{"message":"401 Unauthorized secret=fake-value"}}`, reason: "auth", code: 1},
		{name: "quota", backend: "codex", stdout: `{"type":"turn.failed","error":{"message":"quota exhausted api_key=fake-value"}}`, reason: "quota", code: 1},
		{name: "model", backend: "codex", stdout: `{"type":"turn.failed","error":{"message":"model not found password=fake-value"}}`, reason: "model_mismatch", code: 1},
		{name: "missing", backend: "codex", stderr: "executable file not found", reason: "missing_binary", code: 127},
		{name: "invalid", backend: "codex", stderr: "unrecognized reply token=fake-value", reason: "invalid_output", code: 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			x := &Execution{Config: ReviewerConfig{Backend: c.backend}}
			done := reviewFrom(x, c.code, outputBuffer([]byte(c.stdout), 65536), outputBuffer([]byte(c.stderr), 65536), nil, false)
			if done.Answer.Reason != c.reason || done.Answer.Diagnostic == "" || strings.Contains(done.Answer.Diagnostic, "fake-value") {
				t.Fatalf("bad metadata: %+v", done.Answer)
			}
		})
	}
}

func TestFailedCodexReviewKeepsReportedModel(t *testing.T) {
	_, stderr, _ := recorded(t, "codex", "success")
	done := reviewFrom(&Execution{Config: ReviewerConfig{Backend: "codex"}}, 1, outputBuffer(nil, 65536), outputBuffer(stderr, 65536), nil, false)
	if done.Answer.Model != "gpt-5.6-luna" {
		t.Fatalf("reported model lost: %+v", done.Answer)
	}
}

func TestNewDiagnosticsPreserveStoppedReviewBehavior(t *testing.T) {
	x := &Execution{Config: ReviewerConfig{Backend: "codex"}}
	for _, c := range []struct {
		stderr string
		err    error
		reason string
	}{{stderr: "401 Unauthorized", reason: "auth"}, {err: os.ErrNotExist, reason: "missing_binary"}} {
		done := reviewFrom(x, 1, outputBuffer(nil, 65536), outputBuffer([]byte(c.stderr), 65536), c.err, false)
		if done.Status != "inconclusive" || done.Failure != nil || done.Answer.Reason != c.reason {
			t.Fatalf("diagnostics changed stop behavior: %+v", done)
		}
	}
}

func TestMissingReviewerBeforeLaunchIsDiagnosable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GREENHOUSE_HOME", t.TempDir())
	done := executeReview(context.Background(), &Execution{Config: ReviewerConfig{Backend: "pi", Model: "fixture/model"}})
	if done.Status != "unconfigured" || done.Answer.Reason != "missing_binary" || done.Answer.Diagnostic == "" {
		t.Fatalf("missing binary masked: %+v", done)
	}
}

func TestReviewRoundPreservesReportedModelOnInconclusiveReply(t *testing.T) {
	_, codexStderr, _ := recorded(t, "codex", "success")
	for _, c := range []struct{ name, backend, stdout, stderr, model string }{
		{"incomplete codex", "codex", "", string(codexStderr), "gpt-5.6-luna"},
		{"malformed codex", "codex", "not JSON", string(codexStderr), "gpt-5.6-luna"},
		{"claude error", "claude", "{\"type\":\"system\",\"subtype\":\"init\",\"model\":\"claude-haiku-5-5\"}\n{\"type\":\"result\",\"is_error\":true,\"result\":\"request rejected\"}", "", "claude-haiku-5-5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			x := &Execution{Config: ReviewerConfig{Backend: c.backend}}
			done := reviewFrom(x, 0, outputBuffer([]byte(c.stdout), 65536), outputBuffer([]byte(c.stderr), 65536), nil, false)
			retainReviewMetadata(x, done)
			r := &Run{Host: &Host{Name: "host", ID: "1"}, Review: x}
			if got := hostResult(r, true).Model; got != c.model {
				t.Fatalf("host_result.model=%q; want reported session model %q on inconclusive reply", got, c.model)
			}
			if done.Status != "inconclusive" || done.Failure != nil {
				t.Fatalf("unexpected outcome: %+v", done)
			}
		})
	}
}

func TestReviewRoundClassifiesStructuredZeroExitModelFailure(t *testing.T) {
	stdout := []byte(`{"type":"message_end","message":{"role":"assistant","provider":"fixture","model":"reported-model","stopReason":"error","errorMessage":"404 model not found","content":[]}}`)
	x := &Execution{Config: ReviewerConfig{Backend: "pi"}}
	done := reviewFrom(x, 0, outputBuffer(stdout, 65536), outputBuffer(nil, 65536), nil, false)
	retainReviewMetadata(x, done)
	result := hostResult(&Run{Host: &Host{Name: "host", ID: "1"}, Review: x}, true)
	if result.Reason != "model_mismatch" {
		t.Fatalf("reason=%q diagnostic=%q; want model_mismatch", result.Reason, result.Diagnostic)
	}
	if done.Status != "inconclusive" || done.Failure != nil {
		t.Fatalf("diagnostics changed pause/refund behavior: %+v", done)
	}
	failure := classifyBackend("pi", 0, stdout, nil, time.Now())
	if failure == nil || !failure.diagnosticOnly {
		t.Fatalf("new classification must be diagnostic only: %+v", failure)
	}
	if got := classifyBackend("pi", 0, []byte(`{"type":"message_end","message":{"role":"assistant","stopReason":"stop"}}`), []byte("404 model not found"), time.Now()); got != nil {
		t.Fatalf("successful zero-exit stderr noise classified: %+v", got)
	}
}

package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installAgentCLIs(t *testing.T, home string) string {
	t.Helper()
	root := filepath.Dir(home)
	fixtures, err := filepath.Abs(filepath.Join("..", "..", "testdata", "backends"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(fixtures, "fake-cli.py"))
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Replace(string(source), "FIXTURES", "'"+fixtures+"'", 1)
	for _, name := range []string{"claude", "codex"} {
		writeExecutable(t, filepath.Join(root, "fake-"+name), script)
		native := filepath.Join(root, "native-bin")
		if err := os.MkdirAll(native, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "fake-"+name), filepath.Join(native, name)); err != nil {
			t.Fatal(err)
		}
	}
	writePrivateJSON(t, filepath.Join(home, "backends.json"), map[string]any{
		"claude": map[string]any{"executable": filepath.Join(root, "fake-claude")},
		"codex":  map[string]any{"executable": filepath.Join(root, "fake-codex"), "provider": map[string]any{"base_url": "http://127.0.0.1:1/v1"}},
	})
	return root
}

func agentCalls(t *testing.T, root, backend string) []map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, backend+"-calls.jsonl"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var calls []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var call map[string]any
		if err := json.Unmarshal([]byte(line), &call); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, call)
	}
	return calls
}

func onePassWith(t *testing.T, backend, model string) (string, string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home, v := prepareFixture(t)
	root := installAgentCLIs(t, home)
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop", "no-comments"}, "reviewer_backend": backend, "reviewer_model": model, "fixer_backend": "pi", "fixer_model": "fixture/model"}
	v = mutate(t, bin, repo, home, v, "apply", "switch-"+backend, settings)
	if v["outcome"] != "ok" {
		t.Fatalf("apply %s: %v", backend, v)
	}
	v = mutate(t, bin, repo, home, v, "resolve-rules", "rules-"+backend, nil)
	return bin, repo, home, root, v
}

func TestOnePassReviewThroughClaudeAndCodex(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ backend, model, label string }{
		{"claude", "claude-haiku-5-5", "claude-stream-json-v1"},
		{"codex", "gpt-5.6-luna", "codex-json-v1"},
	} {
		t.Run(c.backend, func(t *testing.T) {
			bin, repo, home, root, v := onePassWith(t, c.backend, c.model)
			v = mutate(t, bin, repo, home, v, "review", "review-"+c.backend, nil)
			if v["outcome"] != "ok" {
				t.Fatalf("review: %v", v)
			}
			v = waitReview(t, bin, repo, home)
			run := runOf(t, v)
			review := run["review"].(map[string]any)
			result, _ := review["result"].(map[string]any)
			if run["status"] != "clean-review" || review["backend"] != c.label || result == nil || result["reported_model"] != c.model || result["reported_provider"] != c.backend {
				t.Fatalf("review through %s: status %v review %v", c.backend, run["status"], review)
			}
			calls := agentCalls(t, root, c.backend)
			if len(calls) != 1 {
				t.Fatalf("calls: %v", calls)
			}
			input, _ := calls[0]["input"].(string)
			for _, definition := range []string{"P0 means data loss", "P1 means wrong behavior", "P2 means an edge case", "P3 means style or maintainability", "Only P0 and P1 findings with valid actions block progression", "P2 and P3 findings do not block"} {
				if !strings.Contains(input, definition) {
					t.Errorf("%s reviewer input lacks %q", c.backend, definition)
				}
			}
			args := strs(calls[0]["args"])
			joined := strings.Join(args, " ")
			for _, required := range map[string][]string{
				"claude": {"-p", "--output-format stream-json", "--json-schema", "--model " + c.model, "--tools Read,Grep,Glob", "--session-id", "--safe-mode", "--strict-mcp-config", "--setting-sources", "--permission-mode dontAsk"},
				"codex":  {"exec", "--json", "--ignore-rules", "--sandbox read-only", "-m " + c.model, "--output-schema ../schema.json", "--disable shell_tool", "--disable hooks", "project_doc_max_bytes=0"},
			}[c.backend] {
				if !strings.Contains(joined, required) {
					t.Errorf("%s argv lacks %q: %v", c.backend, required, args)
				}
			}
			for _, key := range strs(calls[0]["env"]) {
				if strings.HasPrefix(key, "CLAUDECODE") || strings.HasPrefix(key, "CLAUDE_CODE_") || strings.HasPrefix(key, "CODEX_THREAD") {
					t.Errorf("author session variable %s reached the %s reviewer", key, c.backend)
				}
			}
			if calls[0]["cwd"] == repo {
				t.Error("reviewer ran inside the source repository")
			}
		})
	}
}

func TestReviewRefusesMismatchedReportedModel(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := onePassWith(t, "claude", "claude-opus-5-5")
	script, _ := os.ReadFile(filepath.Join(root, "fake-claude"))
	writeExecutable(t, filepath.Join(root, "fake-claude"), strings.Replace(string(script), "model=args[args.index('--model' if '--model' in args else '-m')+1]", "model='claude-haiku-5-5'", 1))
	mutate(t, bin, repo, home, v, "review", "review-mismatch", nil)
	v = waitReview(t, bin, repo, home)
	review := runOf(t, v)["review"].(map[string]any)
	if review["status"] != "inconclusive" || !strings.Contains(review["log"].(string), "model") {
		t.Fatalf("mismatched model must be inconclusive: %v", review)
	}
}

func TestNativeBackendRecoversInvalidVerdict(t *testing.T) {
	for _, c := range []struct{ backend, model string }{{"claude", "claude-haiku-5-5"}, {"codex", "gpt-5.6-luna"}} {
		t.Run(c.backend, func(t *testing.T) {
			bin, repo, home, root, v := onePassWith(t, c.backend, c.model)
			if err := os.WriteFile(filepath.Join(root, "bad-once-"+c.backend), nil, 0600); err != nil {
				t.Fatal(err)
			}
			mutate(t, bin, repo, home, v, "review", "recover-native", nil)
			run := runOf(t, waitReview(t, bin, repo, home))
			review := run["review"].(map[string]any)
			if review["status"] != "clean-review" || review["finalization"] != true {
				t.Fatalf("native recovery: status=%v reason=%v diagnostic=%v", review["status"], review["reason"], review["diagnostic"])
			}
			calls := agentCalls(t, root, c.backend)
			if len(calls) != 2 {
				t.Fatalf("want one recovery turn: %v", calls)
			}
			first, last := strs(calls[0]["args"]), strs(calls[1]["args"])
			if c.backend == "claude" {
				id := ""
				for i, a := range first {
					if a == "--session-id" {
						id = first[i+1]
					}
				}
				if id == "" || !strings.Contains(strings.Join(last, " "), "--resume "+id) {
					t.Fatalf("changed session: %v %v", first, last)
				}
			} else if !strings.Contains(strings.Join(last, " "), "exec resume") {
				t.Fatalf("no resume: %v", last)
			}
		})
	}
}

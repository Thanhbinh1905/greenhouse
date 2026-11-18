package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewDropsOnlyOutOfRangeFinding(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `import builtins
original_print=print
def print(text):
 event=json.loads(text)
 if event.get('type')=='message_end':
  verdict=json.loads(event['message']['content'][0]['text'])
  bad=dict(verdict['findings'][0]); bad['start']=999; bad['end']=999
  verdict['findings'].append(bad)
  bad=dict(verdict['findings'][0]);bad['file']='unrelated.txt';verdict['findings'].append(bad)
  event['message']['content'][0]['text']=json.dumps(verdict)
 original_print(json.dumps(event))
`)
	mutate(t, bin, repo, home, v, "review", "range", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	result, _ := review["result"].(map[string]any)
	if review["status"] != "outstanding-findings" || result == nil || len(result["findings"].([]any)) != 1 || !strings.Contains(review["diagnostic"].(string), "outside changed hunks") {
		t.Fatalf("valid finding must survive with diagnostic: %v", review)
	}
}

func TestReviewRecoversMissingFinalInSameSession(t *testing.T) {
	for _, mode := range []string{"missing", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			bin, repo, home, v := prepareFixture(t)
			installFake(t, home, `session=sys.argv[sys.argv.index('--session')+1]
marker=root+'/review-session'
if '--no-tools' not in sys.argv:
 with open(marker,'w') as f: f.write(session)
 text='not JSON' if '`+mode+`'=='invalid' else ''
 print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop' if text else 'toolUse','content':[{'type':'text','text':text}],'usage':{'input':12,'output':3,'totalTokens':15}}}))
 if text: print(json.dumps({'type':'agent_settled','aborted':False}))
 sys.exit(0)
assert open(marker).read()==session
assert 'Return your verdict now' in b['Controller']
`)
			mutate(t, bin, repo, home, v, "review", "recover", nil)
			settled := runOf(t, waitReview(t, bin, repo, home))
			review := settled["review"].(map[string]any)
			if review["status"] != "outstanding-findings" || review["usage"].(map[string]any)["tokens"] != float64(15) {
				t.Fatalf("missing final must recover: status=%v usage=%v", review["status"], review["usage"])
			}
			attempts := settled["attempts"].([]any)
			if len(attempts) != 1 || attempts[0].(map[string]any)["tokens"] != float64(15) {
				t.Fatalf("attempt usage: %v", attempts)
			}
			calls, _ := os.ReadFile(filepath.Join(filepath.Dir(home), "invocations"))
			if strings.Count(string(calls), "invoked") != 2 {
				t.Fatalf("want exactly one recovery: %s", calls)
			}
		})
	}
}

func TestReviewFinalizationIsBounded(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `if '--no-tools' in sys.argv:
 print('still not a verdict')
 sys.exit(0)
print(json.dumps({'type':'agent_settled','aborted':False}))
sys.exit(0)
`)
	mutate(t, bin, repo, home, v, "review", "bounded-recovery", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	if review["status"] != "inconclusive" || review["finalization"] != true {
		t.Fatalf("bad final must fail after recovery: status=%v finalization=%v", review["status"], review["finalization"])
	}
	calls, _ := os.ReadFile(filepath.Join(filepath.Dir(home), "invocations"))
	if strings.Count(string(calls), "invoked") != 2 {
		t.Fatalf("unexpected calls: %s", calls)
	}
}

func TestReviewBudgetForcesFinalization(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `if '--no-tools' not in sys.argv: time.sleep(30)
`)
	var cfg map[string]any
	data, err := os.ReadFile(filepath.Join(home, "backends.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["review_budget"] = "5s"
	writePrivateJSON(t, filepath.Join(home, "backends.json"), cfg)
	start := time.Now()
	mutate(t, bin, repo, home, v, "review", "budget", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	if review["status"] != "outstanding-findings" || review["finalization"] != true || time.Since(start) > 10*time.Second {
		t.Fatalf("budget did not finalize: status=%v finalization=%v elapsed=%s", review["status"], review["finalization"], time.Since(start))
	}
}

func TestReviewFinalizationDeadlineStopsHungAnswer(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `if '--no-tools' in sys.argv: time.sleep(30)
print(json.dumps({'type':'agent_settled','aborted':False}))
sys.exit(0)
`)
	var cfg map[string]any
	data, _ := os.ReadFile(filepath.Join(home, "backends.json"))
	json.Unmarshal(data, &cfg)
	cfg["review_budget"] = "2s"
	writePrivateJSON(t, filepath.Join(home, "backends.json"), cfg)
	start := time.Now()
	mutate(t, bin, repo, home, v, "review", "hang", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	if review["status"] != "inconclusive" || review["reason"] != "review_budget" || time.Since(start) > 8*time.Second {
		t.Fatalf("hung finalization: status=%v reason=%v elapsed=%s", review["status"], review["reason"], time.Since(start))
	}
}

func TestReviewBudgetConfiguration(t *testing.T) {
	_, _, home := fixture(t)
	t.Setenv("GREENHOUSE_HOME", home)
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		value string
		valid bool
	}{{"", true}, {"250ms", true}, {"1h", true}, {"0s", false}, {"-1m", false}, {"2h", false}, {"later", false}} {
		writePrivateJSON(t, filepath.Join(home, "backends.json"), map[string]any{"review_budget": c.value})
		got, err := reviewBudget()
		if (err == nil) != c.valid {
			t.Fatalf("budget %q: %s %v", c.value, got, err)
		}
		if c.value == "" && got != defaultReviewBudget {
			t.Fatalf("default: %s", got)
		}
	}
}

func TestReviewInputKeepsNumberedHunkContext(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	var lines strings.Builder
	for i := 1; i <= 2000; i++ {
		fmt.Fprintf(&lines, "line %d\n", i)
	}
	os.WriteFile(filepath.Join(repo, "large.txt"), []byte(lines.String()), 0600)
	git(t, repo, "add", "large.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "large base")
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	os.WriteFile(filepath.Join(repo, "large.txt"), []byte(strings.Replace(lines.String(), "line 1000\n", "changed 1000\n", 1)), 0600)
	git(t, repo, "add", "large.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "one line")
	v = mutate(t, bin, repo, home, v, "new", "new-run", nil)
	v = mutate(t, bin, repo, home, v, "apply", "model", map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
	v = axi(t, bin, repo, home, prepareRequest(v, "new-head", target))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	v = mutate(t, bin, repo, home, v, "resolve-rules", "new-rules", nil)
	installFake(t, home, `v={"head":b["Head"],"verdict":"clean","coverage":b["Paths"],"findings":[]}
print(json.dumps({"type":"message_end","message":{"role":"assistant","provider":"fixture","model":"model","stopReason":"stop","content":[{"type":"text","text":json.dumps(v)}]}}))
print(json.dumps({"type":"agent_settled","aborted":False}))
sys.exit(0)
`)
	v = mutate(t, bin, repo, home, v, "review", "excerpt", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("review refused: %v", v)
	}
	settled := waitReview(t, bin, repo, home)
	if runOf(t, settled)["review"] == nil {
		t.Fatalf("no review: %v", settled)
	}
	capture, err := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	if err != nil {
		t.Fatalf("capture unavailable: %v review=%v", err, runOf(t, settled)["review"])
	}
	var data struct {
		Input struct {
			Files        []reviewFile
			ChangedHunks map[string][][2]int
		}
	}
	if err := json.Unmarshal(capture, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Input.Files) != 1 || len(data.Input.Files[0].Lines) > 3000 || !strings.Contains(data.Input.Files[0].Lines, "1000: changed 1000") || !strings.Contains(data.Input.Files[0].Lines, "999: line 999") || len(data.Input.ChangedHunks["large.txt"]) == 0 {
		t.Fatalf("input did not focus on numbered hunks: %+v", data.Input)
	}
}

func TestReviewWithOnlyOutOfRangeFindingProducesVerdict(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `original_print=print
def print(text):
 event=json.loads(text)
 if event.get('type')=='message_end':
  verdict=json.loads(event['message']['content'][0]['text'])
  verdict['findings'][0]['start']=999;verdict['findings'][0]['end']=999
  event['message']['content'][0]['text']=json.dumps(verdict)
 original_print(json.dumps(event))
`)
	mutate(t, bin, repo, home, v, "review", "drop-all", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	if review["status"] != "clean-review" || !strings.Contains(review["diagnostic"].(string), "outside changed hunks") {
		t.Fatalf("all rejected: status=%v diagnostic=%v", review["status"], review["diagnostic"])
	}
}

func TestPrivateClaudeSessionRetainsNativeConfiguration(t *testing.T) {
	native := t.TempDir()
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(native, ".claude.json"), []byte(`{"hasCompletedOnboarding":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", native)
	session, err := newReviewSession(&Execution{Config: ReviewerConfig{Backend: BackendClaude}}, state)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		found := false
		for _, v := range session.env {
			if v == key+"="+session.temp {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s does not use private writable state", key)
		}
	}
	if _, err := os.Stat(filepath.Join(session.state, ".claude.json")); err != nil {
		t.Fatalf("private session lost native onboarding state: %v", err)
	}
}

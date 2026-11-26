package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func TestReviewDropsOnlyOutOfRangeFinding(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestReviewBudgetAfterPartialCoverageSettlesPartial(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	writeFixtureFile(t, repo, "oversized.txt", strings.Repeat("changed content\n", 6000))
	for i := 0; i < 12; i++ {
		writeFixtureFile(t, repo, fmt.Sprintf("path-%02d.txt", i), strings.Repeat(fmt.Sprintf("content %d\n", i), 800))
	}
	commitReviewFixture(t, repo, "large change")
	v := prepareReviewAdmission(t, bin, repo, home, target)
	installFake(t, home, `coverage=b['Paths'][:1]
findings=[{'id':'reported','priority':'P2','origin':'correctness','file':coverage[0],'start':1,'end':1,'explanation':'A reported issue','action':'Correct it','rule':'','head':b['Head']}] if coverage else []
v={'head':b['Head'],'verdict':'findings' if findings else 'clean','coverage':coverage,'findings':findings}
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
sys.stdout.flush()
time.sleep(30)
`)
	var cfg map[string]any
	data, err := os.ReadFile(filepath.Join(home, "backends.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["review_budget"] = "10s"
	writePrivateJSON(t, filepath.Join(home, "backends.json"), cfg)
	mutate(t, bin, repo, home, v, "review", "partial-budget", nil)
	settled := runOf(t, waitReview(t, bin, repo, home))
	review := settled["review"].(map[string]any)
	if review["status"] != "partial-review" {
		calls, _ := os.ReadFile(filepath.Join(filepath.Dir(home), "invocations"))
		t.Fatalf("budget exhaustion after partial coverage must settle partial: status=%v reason=%v finalization=%v diagnostic=%v log=%v calls=%q", review["status"], review["reason"], review["finalization"], review["diagnostic"], review["log"], calls)
	}
	result := review["result"].(map[string]any)
	if result["verdict"] != "partial" {
		t.Fatalf("budget exhaustion must yield a partial verdict: %v", result)
	}
	if findings := result["findings"].([]any); len(findings) != 1 {
		t.Fatalf("findings raised before budget exhaustion must be retained: %v", findings)
	}
	uncovered := result["uncovered"].([]any)
	if len(uncovered) != 12 {
		t.Fatalf("every unreviewed changed path must be listed: %v", uncovered)
	}
	reasons := map[string]bool{}
	for _, item := range uncovered {
		entry := item.(map[string]any)
		reason := entry["reason"].(string)
		reasons[reason] = true
		if reason != "hunk windows exceed 65536 bytes" && !strings.HasPrefix(reason, "prompt exceeds model context (") && reason != "review_budget" {
			t.Fatalf("unexpected uncovered reason: %v", entry)
		}
	}
	if !reasons["review_budget"] || !reasons["hunk windows exceed 65536 bytes"] {
		t.Fatalf("uncovered paths must include the budget and hunk-window reasons: %v", uncovered)
	}
	promptReason := false
	for reason := range reasons {
		promptReason = promptReason || strings.HasPrefix(reason, "prompt exceeds model context (")
	}
	if !promptReason {
		t.Fatalf("large input must leave paths uncovered by the prompt budget: %v", uncovered)
	}
}

func TestReviewFinalizationOmitsInitiallyUnreviewablePaths(t *testing.T) {
	x := &Execution{Input: []byte("{\"Head\":\"head\",\"Paths\":[\"reviewable\",\"excluded\"],\"Contract\":\"contract\"}\n\nDiff:\nchange"), Uncovered: []UncoveredPath{{Path: "excluded", Reason: "hunk windows exceed 65536 bytes"}}}
	input, err := reviewFinalizationInput(x, reviewOutcome{}, true)
	if err != nil {
		t.Fatal(err)
	}
	header, _, ok := strings.Cut(string(input), "\n\nDiff:\n")
	if !ok {
		t.Fatalf("finalization input lacks diff: %s", input)
	}
	var fields struct {
		Paths []string
	}
	if err := json.Unmarshal([]byte(header), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields.Paths) != 1 || fields.Paths[0] != "reviewable" {
		t.Fatalf("finalization Paths = %v; want only reviewable paths", fields.Paths)
	}
}

func TestReviewBudgetFinalizationKeepsCompleteVerdict(t *testing.T) {
	for _, test := range []struct {
		name, verdict, status string
	}{
		{"clean", "clean", "clean-review"},
		{"findings", "findings", "outstanding-findings"},
		{"empty change", "clean", "clean-review"},
	} {
		t.Run(test.name, func(t *testing.T) {
			coverage := []string{"a"}
			if test.name == "empty change" {
				coverage = []string{}
			}
			done := reviewOutcome{Status: "inconclusive", Result: &Verdict{Verdict: test.verdict, Coverage: coverage}}
			got := settleReviewBudget(done, reviewOutcome{})
			if got.Status != test.status || got.Result == nil || got.Result.Verdict != test.verdict || got.Answer.Reason != "" {
				t.Fatalf("complete verdict at budget expiry: %+v", got)
			}
		})
	}
}

func TestSettleReviewBudgetMakesIncompleteVerdictPartial(t *testing.T) {
	for _, coverage := range [][]string{{"a"}, {}} {
		done := reviewOutcome{Status: "outstanding-findings", Result: &Verdict{Verdict: "findings", Coverage: coverage, Uncovered: []UncoveredPath{{Path: "b", Reason: "not covered by reviewer within budget"}}}}
		got := settleReviewBudget(done, reviewOutcome{})
		if got.Status != "partial-review" || got.Result.Verdict != "partial" || got.Result.Uncovered[0].Reason != "review_budget" || got.Answer.Reason != "" {
			t.Fatalf("coverage=%v incomplete verdict at budget expiry: %+v", coverage, got)
		}
	}
}

func TestSettleReviewBudgetClearsMissingVerdict(t *testing.T) {
	got := settleReviewBudget(reviewOutcome{Status: "running"}, reviewOutcome{})
	if got.Status != "inconclusive" || got.Result != nil || got.Answer.Reason != "review_budget" {
		t.Fatalf("missing result at budget expiry: %+v", got)
	}
}

func TestReviewBudgetWithoutFinalizationKeepsCompleteVerdict(t *testing.T) {
	for _, verdict := range []string{"clean", "findings"} {
		done := settleReviewBudget(reviewOutcome{Status: "inconclusive", Result: &Verdict{Verdict: verdict, Coverage: []string{"a"}}}, reviewOutcome{})
		want := "clean-review"
		if verdict == "findings" {
			want = "outstanding-findings"
		}
		if done.Status != want || done.Result.Verdict != verdict {
			t.Fatalf("verdict %q without finalization: status=%q result=%+v", verdict, done.Status, done.Result)
		}
	}
}

func TestSettleReviewBudgetHostResultNeverDisagrees(t *testing.T) {
	for _, test := range []struct {
		name, status, verdict string
		result                *Verdict
	}{
		{"complete clean", "clean-review", "clean", &Verdict{Verdict: "clean", Coverage: []string{"a"}}},
		{"complete findings", "outstanding-findings", "findings", &Verdict{Verdict: "findings", Coverage: []string{"a"}}},
		{"incomplete", "partial-review", "partial", &Verdict{Verdict: "partial", Coverage: []string{"a"}, Uncovered: []UncoveredPath{{Path: "b", Reason: "review_budget"}}}},
		{"no result", "inconclusive", "", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			done := settleReviewBudget(reviewOutcome{Status: "inconclusive", Result: test.result}, reviewOutcome{})
			run := &Run{Host: &Host{Name: "host", ID: "test"}, Candidate: "head", Target: "target", Review: &Execution{ID: "review", Status: done.Status, Result: done.Result}}
			host := hostResult(run, true)
			if done.Status != test.status || host.Status != test.status || host.Verdict != test.verdict {
				t.Fatalf("run status=%q host status=%q verdict=%q; want %q/%q", done.Status, host.Status, host.Verdict, test.status, test.verdict)
			}
		})
	}
}

func TestHostResultKeepsInconclusiveStatusWithCarriedFindings(t *testing.T) {
	run := &Run{
		Host:      &Host{Name: "host", ID: "test"},
		Candidate: "head",
		Target:    "target",
		Review:    &Execution{Status: "inconclusive"},
		HostCarried: []CarriedFinding{{
			Finding: Finding{ID: "inherited", File: "sample.txt", Start: 1, End: 1},
			Kept:    true,
		}},
	}
	host := hostResult(run, true)
	if host.Status != "inconclusive" || host.Verdict != "" || len(host.Findings) != 1 {
		t.Fatalf("host status=%q verdict=%q findings=%v; want inconclusive, no verdict, and carried finding", host.Status, host.Verdict, host.Findings)
	}
}

func TestHostResultKeepsPartialStatusWithCarriedFindings(t *testing.T) {
	run := &Run{
		Host:      &Host{Name: "host", ID: "test"},
		Candidate: "head",
		Target:    "target",
		Review: &Execution{
			Status: "partial-review",
			Result: &Verdict{Verdict: "partial", Uncovered: []UncoveredPath{{Path: "other.txt"}}},
		},
		HostCarried: []CarriedFinding{{
			Finding: Finding{ID: "inherited", File: "sample.txt", Start: 1, End: 1},
			Kept:    true,
		}},
	}
	host := hostResult(run, true)
	if host.Status != "partial-review" || host.Verdict != "partial" || len(host.Findings) != 1 {
		t.Fatalf("host status=%q verdict=%q findings=%v; want partial-review, partial, and carried finding", host.Status, host.Verdict, host.Findings)
	}
}

func TestHostResultKeepsPausedStatusWithCarriedFindings(t *testing.T) {
	run := &Run{
		Host:      &Host{Name: "host", ID: "test"},
		Candidate: "head",
		Target:    "target",
		Review:    &Execution{Status: "paused"},
		HostCarried: []CarriedFinding{{
			Finding: Finding{ID: "inherited", File: "sample.txt", Start: 1, End: 1},
			Kept:    true,
		}},
	}
	host := hostResult(run, true)
	if host.Status != "paused" || host.Verdict != "" || len(host.Findings) != 1 {
		t.Fatalf("host status=%q verdict=%q findings=%v; want paused, no verdict, and carried finding", host.Status, host.Verdict, host.Findings)
	}
}

func TestSettleReviewBudgetOutcomeMatrix(t *testing.T) {
	outcomes := map[string]reviewOutcome{
		"none":           {},
		"invalid":        {Status: "inconclusive", Answer: agentAnswer{Reason: "invalid_output"}},
		"partial":        {Status: "partial-review", Result: &Verdict{Head: "exploration", Verdict: "partial", Coverage: []string{"a"}, Uncovered: []UncoveredPath{{Path: "b", Reason: "review_budget"}}, Findings: []Finding{{ID: "exploration"}}}},
		"complete":       {Status: "outstanding-findings", Result: &Verdict{Head: "exploration", Verdict: "findings", Coverage: []string{"a"}, Findings: []Finding{{ID: "exploration"}}}},
		"inconclusive":   {Status: "inconclusive", Result: &Verdict{Verdict: "inconclusive", Coverage: []string{"a"}, Findings: []Finding{}}},
		"final partial":  {Status: "partial-review", Result: &Verdict{Head: "finalization", Verdict: "partial", Coverage: []string{"a", "b"}, Uncovered: []UncoveredPath{{Path: "c", Reason: "review_budget"}}, Findings: []Finding{{ID: "finalization"}}}},
		"final complete": {Status: "clean-review", Result: &Verdict{Head: "finalization", Verdict: "clean", Coverage: []string{"a"}, Findings: []Finding{}}},
		"timeout":        {Status: "cancelled", Answer: agentAnswer{Reason: "timeout"}},
	}
	finalNames := []string{"none", "invalid", "inconclusive", "final partial", "final complete", "timeout"}
	matrix := []struct {
		exploration string
		statuses    []string
		heads       []string
	}{
		{"none", []string{"inconclusive", "inconclusive", "inconclusive", "partial-review", "clean-review", "inconclusive"}, []string{"", "", "", "finalization", "finalization", ""}},
		{"invalid", []string{"inconclusive", "inconclusive", "inconclusive", "partial-review", "clean-review", "inconclusive"}, []string{"", "", "", "finalization", "finalization", ""}},
		{"partial", []string{"partial-review", "partial-review", "partial-review", "partial-review", "clean-review", "partial-review"}, []string{"exploration", "exploration", "exploration", "finalization", "finalization", "exploration"}},
		{"complete", []string{"outstanding-findings", "outstanding-findings", "outstanding-findings", "outstanding-findings", "clean-review", "outstanding-findings"}, []string{"exploration", "exploration", "exploration", "exploration", "finalization", "exploration"}},
	}
	for _, row := range matrix {
		for i, finalName := range finalNames {
			t.Run(row.exploration+" + "+finalName, func(t *testing.T) {
				got := settleReviewBudget(outcomes[finalName], outcomes[row.exploration])
				if got.Status != row.statuses[i] {
					t.Fatalf("settled status=%q; want %q", got.Status, row.statuses[i])
				}
				if row.heads[i] == "" {
					if got.Result != nil {
						t.Fatalf("settled result=%+v; want none", got.Result)
					}
					return
				}
				if got.Result == nil || got.Result.Head != row.heads[i] {
					t.Fatalf("settled result=%+v; want %s result", got.Result, row.heads[i])
				}
			})
		}
	}
}

func TestReviewFinalizationGetsHalfOfReviewBudget(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `if '--no-tools' not in sys.argv:
 time.sleep(30)
`)
	scriptPath := filepath.Join(filepath.Dir(home), "fake-pi")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script = []byte(strings.Replace(string(script), "if '--no-tools' not in sys.argv:\n time.sleep(30)\n", "if '--no-tools' not in sys.argv:\n time.sleep(30)\nelse:\n time.sleep(2.5)\n", 1))
	if err := os.WriteFile(scriptPath, script, 0700); err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	data, err := os.ReadFile(filepath.Join(home, "backends.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["review_budget"] = "10s"
	writePrivateJSON(t, filepath.Join(home, "backends.json"), cfg)
	started := time.Now()
	mutate(t, bin, repo, home, v, "review", "half-budget-finalization", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	if review["status"] != "outstanding-findings" || review["finalization"] != true || time.Since(started) > 10*time.Second {
		t.Fatalf("finalization did not receive enough time to return a verdict: status=%v finalization=%v elapsed=%s log=%v", review["status"], review["finalization"], time.Since(started), review["log"])
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
	data, err := os.ReadFile(filepath.Join(home, "backends.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["review_budget"] = "2s"
	writePrivateJSON(t, filepath.Join(home, "backends.json"), cfg)
	start := time.Now()
	mutate(t, bin, repo, home, v, "review", "hang", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	if review["status"] != "inconclusive" || review["reason"] != "review_budget" || time.Since(start) > 8*time.Second {
		t.Fatalf("hung finalization: status=%v reason=%v elapsed=%s", review["status"], review["reason"], time.Since(start))
	}
}

func TestReviewBudgetRestartKeepsCompleteVerdict(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `if '--no-tools' in sys.argv:
 v={'head':b['Head'],'verdict':'inconclusive','coverage':[],'findings':[]}
 print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}), flush=True)
 print(json.dumps({'type':'agent_settled','aborted':False}), flush=True)
 time.sleep(30)
else:
 v={'head':b['Head'],'verdict':'findings','coverage':b['Paths'],'findings':[{'id':'restart','priority':'P2','origin':'correctness','file':b['Paths'][0],'start':1,'end':1,'explanation':'A review finding','action':'Inspect the change','rule':'','head':b['Head']}]}
 print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}), flush=True)
 print(json.dumps({'type':'agent_settled','aborted':False}), flush=True)
 time.sleep(30)
`)
	var cfg map[string]any
	data, err := os.ReadFile(filepath.Join(home, "backends.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["review_budget"] = "2s"
	writePrivateJSON(t, filepath.Join(home, "backends.json"), cfg)
	v = mutate(t, bin, repo, home, v, "review", "restart-budget-review", nil)
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	statePath := filepath.Join(home, "state.json")
	stateBeforeReviewCompletion, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	settled := waitReview(t, bin, repo, home)
	settledRun := runOf(t, settled)
	settledReview := settledRun["review"].(map[string]any)
	if settledRun["status"] != "outstanding-findings" || settledReview["status"] != "outstanding-findings" {
		t.Fatalf("live settlement discarded the exploration verdict: run=%v review=%v", settledRun["status"], settledReview["status"])
	}
	reviewID := settledReview["id"].(string)
	executors := runner.ObserveExecutors(reviewID, func() (string, error) { return home, nil })
	if len(executors) != 2 {
		t.Fatalf("want exploration and finalization executor records, got %d", len(executors))
	}
	var legacyState map[string]any
	if err := json.Unmarshal(stateBeforeReviewCompletion, &legacyState); err != nil {
		t.Fatal(err)
	}
	legacyRuns := legacyState["runs"].(map[string]any)
	legacyRecordFound := false
	for _, value := range legacyRuns {
		storedRun := value.(map[string]any)
		storedReview, _ := storedRun["review"].(map[string]any)
		if storedReview["id"] == reviewID {
			storedReview["review_budget"] = ""
			legacyRecordFound = true
			break
		}
	}
	if !legacyRecordFound {
		t.Fatalf("review %s not found in recovery snapshot", reviewID)
	}
	stateBeforeReviewCompletion, err = json.Marshal(legacyState)
	if err != nil {
		t.Fatal(err)
	}
	stopDaemonAndWait(t, home)
	if err := os.WriteFile(statePath, stateBeforeReviewCompletion, 0600); err != nil {
		t.Fatal(err)
	}
	recovered := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	run := runOf(t, recovered)
	review := run["review"].(map[string]any)
	result, _ := review["result"].(map[string]any)
	if run["status"] != "outstanding-findings" || review["status"] != "outstanding-findings" || result == nil || result["verdict"] != "findings" {
		t.Fatalf("restarted daemon discarded the budget-expiry verdict: run=%v review=%v", run["status"], review)
	}
}

func TestReviewBudgetExpiredUsesLegacyDefaultAndExecutorLimit(t *testing.T) {
	for _, test := range []struct {
		name          string
		budget        string
		executorLimit time.Duration
		stopErr       error
		want          bool
	}{
		{"legacy budget shorter than executor", "", 45 * time.Minute, context.DeadlineExceeded, true},
		{"legacy budget not shorter than executor", "", time.Minute, context.DeadlineExceeded, false},
		{"explicit budget shorter than executor", "30s", time.Minute, context.DeadlineExceeded, true},
		{"cancelled without deadline", "", 45 * time.Minute, context.Canceled, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := reviewBudgetExpired(test.budget, test.executorLimit, test.stopErr); got != test.want {
				t.Fatalf("reviewBudgetExpired(%q, %s, %v) = %t; want %t", test.budget, test.executorLimit, test.stopErr, got, test.want)
			}
		})
	}
}

func TestReviewPassConfiguration(t *testing.T) {
	_, _, home := fixture(t)
	t.Setenv("GREENHOUSE_HOME", home)
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		value int
		valid bool
		want  int
	}{{0, true, 1}, {1, true, 1}, {2, true, 2}, {3, true, 3}, {-1, false, 0}, {4, false, 0}} {
		writePrivateJSON(t, filepath.Join(home, "backends.json"), map[string]any{"review_passes": test.value})
		config, err := trustedBackends()
		if (err == nil) != test.valid {
			t.Fatalf("review_passes %d: %v", test.value, err)
		}
		if err == nil {
			got := config.ReviewPasses
			if got == 0 {
				got = 1
			}
			if got != test.want {
				t.Fatalf("review_passes %d defaults to %d; want %d", test.value, got, test.want)
			}
		}
	}
}

func TestReviewShardConfiguration(t *testing.T) {
	_, _, home := fixture(t)
	t.Setenv("GREENHOUSE_HOME", home)
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		value any
		want  bool
		valid bool
	}{
		{name: "default", value: nil, want: false, valid: true},
		{name: "enabled", value: true, want: true, valid: true},
		{name: "disabled", value: false, want: false, valid: true},
		{name: "invalid type", value: "true", valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := map[string]any{}
			if test.value != nil {
				config["review_shards"] = test.value
			}
			writePrivateJSON(t, filepath.Join(home, "backends.json"), config)
			got, err := trustedBackends()
			if (err == nil) != test.valid {
				t.Fatalf("trustedBackends() error = %v, valid %t", err, test.valid)
			}
			if err == nil && got.ReviewShards != test.want {
				t.Fatalf("ReviewShards = %t, want %t", got.ReviewShards, test.want)
			}
		})
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
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	var lines strings.Builder
	for i := 1; i <= 2000; i++ {
		fmt.Fprintf(&lines, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(repo, "large.txt"), []byte(lines.String()), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "large.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "large base")
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "large.txt"), []byte(strings.Replace(lines.String(), "line 1000\n", "changed 1000\n", 1)), 0600); err != nil {
		t.Fatal(err)
	}
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

func TestReviewClampsFindingPastChangedHunk(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `original_print=print
def print(text):
 event=json.loads(text)
 if event.get('type')=='message_end':
  verdict=json.loads(event['message']['content'][0]['text'])
  verdict['findings'][0]['end']=999
  event['message']['content'][0]['text']=json.dumps(verdict)
 original_print(json.dumps(event))
`)
	mutate(t, bin, repo, home, v, "review", "drop-all", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	result, _ := review["result"].(map[string]any)
	findings, _ := result["findings"].([]any)
	if review["status"] != "outstanding-findings" || len(findings) != 1 || findings[0].(map[string]any)["start"] != float64(1) || findings[0].(map[string]any)["end"] != float64(1) {
		t.Fatalf("overlapping finding was not clamped and retained: status=%v result=%v", review["status"], result)
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

func TestReviewExecutorTimeoutIsSharedWithFinalization(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `if '--no-tools' not in sys.argv:
 time.sleep(0.6)
 print(json.dumps({'type':'agent_settled','aborted':False}))
 sys.exit(0)
time.sleep(30)
`)
	configs := runOf(t, v)["configs"].([]any)
	settings := configs[len(configs)-1].(map[string]any)["settings"].(map[string]any)
	settings["executor_timeout"] = "2s"
	v = mutate(t, bin, repo, home, v, "apply", "attempt-timeout", settings)
	v = mutate(t, bin, repo, home, v, "resolve-rules", "timeout-rules", nil)
	mutate(t, bin, repo, home, v, "review", "shared-timeout", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	if review["finalization"] != true || review["reason"] != "timeout" {
		t.Fatalf("finalization did not share the effective deadline: status=%v reason=%v finalization=%v", review["status"], review["reason"], review["finalization"])
	}
}

func TestPrivateNativeConfigurationCannotWriteThroughWithoutLandlock(t *testing.T) {
	for _, c := range []struct{ backend, model, dir, file, key string }{
		{"claude", "claude-haiku-5-5", ".claude", ".credentials.json", "CLAUDE_CONFIG_DIR"},
		{"codex", "gpt-5.6-luna", ".codex", "auth.json", "CODEX_HOME"},
	} {
		t.Run(c.backend, func(t *testing.T) {
			bin, repo, home, root, _ := onePassWith(t, c.backend, c.model)
			native := filepath.Join(root, c.dir)
			if err := os.MkdirAll(native, 0700); err != nil {
				t.Fatal(err)
			}
			original := filepath.Join(native, c.file)
			if err := os.WriteFile(original, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(root, "fake-"+c.backend)
			data, err := os.ReadFile(script)
			if err != nil {
				t.Fatal(err)
			}
			probe := "stdin=sys.stdin.read()\nprivate=os.path.join(os.environ['" + c.key + "'],'" + c.file + "')\nos.chmod(private,0o600)\nopen(private,'w').write('modified')\n"
			if err := os.WriteFile(script, []byte(strings.Replace(string(data), "stdin=sys.stdin.read()", probe, 1)), 0700); err != nil {
				t.Fatal(err)
			}
			stopDaemon(t, home)
			v := axiWithoutLandlock(t, bin, repo, home, `{"version":1,"op":"attach"}`)
			mutate(t, bin, repo, home, v, "review", "private-config", nil)
			review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
			if review["status"] != "clean-review" {
				t.Fatalf("review did not complete: %v", review["status"])
			}
			data, err = os.ReadFile(original)
			if err != nil || string(data) != "original" {
				t.Fatalf("native credentials modified through private session: %q %v", data, err)
			}
		})
	}
}

func TestClaudePrivateHomeDoesNotReadOrModifyOriginalOnboarding(t *testing.T) {
	bin, repo, home, root, _ := onePassWith(t, "claude", "claude-haiku-5-5")
	native := filepath.Join(root, ".claude")
	if err := os.MkdirAll(native, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", native)
	if err := os.WriteFile(filepath.Join(native, ".claude.json"), []byte("private onboarding"), 0600); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(root, ".claude.json")
	if err := os.WriteFile(original, []byte("original onboarding"), 0600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "fake-claude")
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	probe := `stdin=sys.stdin.read()
onboarding=os.path.join(os.environ['HOME'],'.claude.json')
with open(root+'/onboarding-read','w') as f: f.write(open(onboarding).read())
with open(onboarding,'w') as f: f.write('reviewer update')
`
	if err = os.WriteFile(script, []byte(strings.Replace(string(data), "stdin=sys.stdin.read()", probe, 1)), 0700); err != nil {
		t.Fatal(err)
	}
	stopDaemon(t, home)
	v := axiWithoutLandlock(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	mutate(t, bin, repo, home, v, "review", "private-home", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	if review["status"] != "clean-review" {
		t.Fatalf("review did not complete: %v", review["status"])
	}
	read, err := os.ReadFile(filepath.Join(root, "onboarding-read"))
	if err != nil || string(read) != "private onboarding" {
		t.Fatalf("reviewer read original home onboarding instead of private copy: %q %v", read, err)
	}
	data, err = os.ReadFile(original)
	if err != nil || string(data) != "original onboarding" {
		t.Fatalf("reviewer modified original home onboarding: %q %v", data, err)
	}
}

func TestDeletionReviewAnchorsFindingsAndContextToCandidate(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	git(t, repo, "config", "diff.context", "0")
	var before, after strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&before, "line %d\n", i)
		if i < 21 || i > 80 {
			fmt.Fprintf(&after, "line %d\n", i)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte(before.String()), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "deletion base")
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte(after.String()), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "remove middle lines")
	v = mutate(t, bin, repo, home, v, "new", "deletion-run", nil)
	v = axi(t, bin, repo, home, prepareRequest(v, "deletion-head", target))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	v = mutate(t, bin, repo, home, v, "resolve-rules", "deletion-rules", nil)
	installFake(t, home, `original_print=print
def print(text):
 event=json.loads(text)
 if event.get('type')=='message_end':
  verdict=json.loads(event['message']['content'][0]['text'])
  anchor=dict(verdict['findings'][0]);anchor['start']=20;anchor['end']=20
  unrelated=dict(anchor);unrelated['start']=60;unrelated['end']=60
  verdict['findings']=[anchor,unrelated]
  event['message']['content'][0]['text']=json.dumps(verdict)
 original_print(json.dumps(event))
`)
	mutate(t, bin, repo, home, v, "review", "deletion-review", nil)
	review := runOf(t, waitReviewWithFindings(t, bin, repo, home))["review"].(map[string]any)
	result := review["result"].(map[string]any)
	findings := result["findings"].([]any)
	if len(findings) != 1 || findings[0].(map[string]any)["start"] != float64(20) {
		t.Fatalf("deletion must keep candidate anchor 20 and drop unrelated candidate line 60: %v", findings)
	}
	if !strings.Contains(review["diagnostic"].(string), "outside changed hunks") {
		t.Fatal("missing dropped-finding diagnostic")
	}
	capture, err := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Input struct {
			Files        []reviewFile
			ChangedHunks map[string][][2]int
		}
	}
	if err = json.Unmarshal(capture, &data); err != nil {
		t.Fatal(err)
	}
	spans := data.Input.ChangedHunks["sample.txt"]
	if len(spans) != 1 || spans[0] != [2]int{20, 20} {
		t.Fatalf("wrong candidate deletion range: %v", spans)
	}
	if len(data.Input.Files) != 1 || !strings.Contains(data.Input.Files[0].Lines, "40: line 100\n") || strings.Contains(data.Input.Files[0].Lines, "41: line 101\n") {
		t.Fatalf("deletion context must end at candidate line 40: %+v", data.Input.Files)
	}
}

func TestFinalizationKeepsExplorationModel(t *testing.T) {
	for _, mode := range []string{"timeout", "no-model"} {
		t.Run(mode, func(t *testing.T) {
			bin, repo, home, v := prepareFixture(t)
			installFake(t, home, `if '--no-tools' in sys.argv:
 if '`+mode+`' == 'timeout': time.sleep(30)
 sys.exit(0)
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':'not JSON'}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
sys.exit(0)
`)
			configs := runOf(t, v)["configs"].([]any)
			settings := configs[len(configs)-1].(map[string]any)["settings"].(map[string]any)
			settings["executor_timeout"] = "4s"
			v = mutate(t, bin, repo, home, v, "apply", "metadata-timeout", settings)
			v = mutate(t, bin, repo, home, v, "resolve-rules", "metadata-rules", nil)
			mutate(t, bin, repo, home, v, "review", "metadata-review", nil)
			data, err := json.Marshal(runOf(t, waitReview(t, bin, repo, home)))
			if err != nil {
				t.Fatal(err)
			}
			var run Run
			if err := json.Unmarshal(data, &run); err != nil {
				t.Fatal(err)
			}
			run.Host = &Host{Name: "fixture", ID: "metadata"}
			run.Review.Config.Model = ""
			host := hostResult(&run, true)
			if !run.Review.Finalization || run.Review.ReportedModel != "model" || host.Model != "model" {
				t.Fatalf("finalization lost exploration model: finalization=%t review model=%q host model=%q reason=%q", run.Review.Finalization, run.Review.ReportedModel, host.Model, run.Review.Reason)
			}
			if mode == "timeout" && run.Review.Reason != "timeout" {
				t.Fatalf("finalization did not time out: %s", run.Review.Reason)
			}
		})
	}
}

func TestFinalReviewAnswerMetadata(t *testing.T) {
	t.Parallel()
	first := agentAnswer{Provider: "exploration-provider", Model: "exploration-model", Usage: TokenUsage{Tokens: 12}}
	for _, final := range []agentAnswer{{}, {Provider: "final-provider", Model: "final-model"}, {Provider: "final-provider"}, {Model: "final-model"}} {
		got := finalReviewAnswer(first, final)
		wantProvider, wantModel := final.Provider, final.Model
		if wantProvider == "" {
			wantProvider = first.Provider
		}
		if wantModel == "" {
			wantModel = first.Model
		}
		if got.Provider != wantProvider || got.Model != wantModel || got.Tokens != 12 {
			t.Fatalf("metadata merge: got %+v, want provider=%s model=%s tokens=12", got, wantProvider, wantModel)
		}
	}
}

func TestReviewToolCallBudgetStopsExploration(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `if '--no-tools' not in sys.argv:
 with open(root+'/explored.json','w') as f: json.dump(b,f)
 for turn in range(300):
  for call in range(2):
   print(json.dumps({'type':'tool_execution_start','toolCallId':'%d-%d'%(turn,call),'toolName':'read','args':{'path':'sample.txt'}}),flush=True)
  print(json.dumps({'type':'turn_end'}),flush=True)
  time.sleep(0.1)
 sys.exit(0)
`)
	start := time.Now()
	mutate(t, bin, repo, home, v, "review", "tool-budget", nil)
	review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
	if review["status"] != "outstanding-findings" || review["finalization"] != true || time.Since(start) > 10*time.Second {
		t.Fatalf("spent tool-call budget must finalize: status=%v finalization=%v elapsed=%s", review["status"], review["finalization"], time.Since(start))
	}
	if !strings.Contains(review["log"].(string), "tool-call budget 6 spent") {
		t.Fatalf("log must name the spent budget: %v", review["log"])
	}
	var explored struct{ ToolCallBudget int }
	data, err := os.ReadFile(filepath.Join(filepath.Dir(home), "explored.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &explored); err != nil || explored.ToolCallBudget != 6 {
		t.Fatalf("review input must carry the budget: %s %v", data, err)
	}
}

func TestLineWatchBoundsPartialLine(t *testing.T) {
	t.Parallel()
	output := runner.NewBuffer(32)
	cancelled := false
	watch := &lineWatch{w: output, cancel: func() { cancelled = true }}
	if _, err := watch.Write([]byte(strings.Repeat("x", 64*1024+1))); err != nil {
		t.Fatal(err)
	}
	if len(watch.partial) > 64*1024 {
		t.Fatalf("partial line retained %d bytes, want at most 65536", len(watch.partial))
	}
	if !output.Exceeded() {
		t.Fatal("oversized stdout must reach the bounded output writer")
	}
	for range 3 {
		if _, err := watch.Write([]byte("\nwaiting for network\n")); err != nil {
			t.Fatal(err)
		}
	}
	if !cancelled {
		t.Fatal("stall events after an oversized line must still be observed")
	}
}

func TestToolCallBudgetScalesWithChangedPaths(t *testing.T) {
	for _, c := range []struct{ paths, want int }{{1, 6}, {3, 6}, {11, 13}, {14, 16}, {38, 40}, {140, 40}} {
		if got := toolCallBudget(c.paths); got != c.want {
			t.Fatalf("%d changed paths: budget %d, want %d", c.paths, got, c.want)
		}
	}
}

func TestToolCallWatchBoundsPartialLine(t *testing.T) {
	output := runner.NewBuffer(32)
	cancelled := false
	watch := &toolCallWatch{w: output, budget: 1, cancel: func() { cancelled = true }}
	oversized := []byte(`{"type":"tool_execution_start","padding":"` + strings.Repeat("x", 64*1024) + `"}`)
	if _, err := watch.Write(oversized); err != nil {
		t.Fatal(err)
	}
	if len(watch.partial) > 64*1024 {
		t.Fatalf("partial line retained %d bytes, want at most 65536", len(watch.partial))
	}
	if !output.Exceeded() {
		t.Fatal("oversized stdout must reach the bounded output writer")
	}
	if _, err := watch.Write([]byte(strings.Repeat("y", 64*1024))); err != nil {
		t.Fatal(err)
	}
	if len(watch.partial) > 64*1024 {
		t.Fatalf("continued partial line retained %d bytes, want at most 65536", len(watch.partial))
	}
	if _, err := watch.Write([]byte("\n{\"type\":\"turn_end\"}\n")); err != nil {
		t.Fatal(err)
	}
	if cancelled {
		t.Fatal("oversized event must not count as a tool call")
	}
	if _, err := watch.Write([]byte("{\"type\":\"tool_execution_start\"}\n{\"type\":\"turn_end\"}\n")); err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("valid events after an oversized line must still count")
	}
	if output.Len() > 32 {
		t.Fatalf("bounded output retained %d bytes, want at most 32", output.Len())
	}
}

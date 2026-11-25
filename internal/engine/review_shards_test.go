package engine

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const shardFakeReviewer = `#!/usr/bin/python3
import sys,json,os
if '--list-models' in sys.argv:
 print('provider model context')
 print('fixture model 128K')
 sys.exit(0)
root=os.path.dirname(__file__)
mode=open(root+'/shard-mode').read().split()
raw=sys.stdin.read()
nl=chr(10)
def answer(v):
 print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
 print(json.dumps({'type':'agent_settled','aborted':False}))
def log(kind):
 with open(root+'/shard-calls','a') as f: f.write(kind+nl)
if nl+nl+'Diff:'+nl not in raw:
 b=json.loads(raw)
 log('plan')
 assert b['MaxShards']==6 and '--no-tools' in sys.argv and 'Add three features' in b['Intent']
 assert sorted(p['Path'] for p in b['Paths'])==['alpha/a1.txt','alpha/a2.txt','beta/b1.txt','beta/b2.txt','gamma/g1.txt']
 shards=[{'name':'Alpha','summary':'adds alpha','paths':['alpha/a1.txt','alpha/a2.txt']},{'name':'Beta','summary':'adds beta','paths':['beta/b1.txt','beta/b2.txt']},{'name':'Gamma','summary':'adds gamma','paths':['gamma/g1.txt']}]
 if 'invalid' in mode: shards[2]['paths'].append('alpha/a1.txt')
 answer({'shards':shards})
 sys.exit(0)
b=json.loads(raw.split(nl+nl+'Diff:'+nl,1)[0])
shard=b.get('Shard') or {}
kind='boundary' if shard.get('Boundary') else 'shard:'+shard['Name'] if shard.get('Name') else 'whole'
if 'confirmation pass' in b['Contract']: kind='confirm:'+kind
log(kind+' '+','.join(b['Paths']))
findings=[]
if 'finding' in mode and 'beta/b1.txt' in b['Paths'] and not shard.get('Boundary'):
 findings=[{'id':'x','priority':'P1','origin':'correctness','file':'beta/b1.txt','start':1,'end':1,'explanation':'Beta breaks','action':'Fix beta','rule':'','head':b['Head']}]
answer({'head':b['Head'],'verdict':'findings' if findings else 'clean','coverage':b['Paths'],'findings':findings})
`

func shardFixture(t *testing.T, mode string) (map[string]any, []string, string) {
	t.Helper()
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	for _, path := range []string{"alpha/a1.txt", "alpha/a2.txt", "beta/b1.txt", "beta/b2.txt", "gamma/g1.txt"} {
		if err := os.MkdirAll(filepath.Join(repo, filepath.Dir(path)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, path), []byte(path+" content\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "add", ".")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "three features")
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	installFake(t, home, "")
	root := filepath.Dir(home)
	if err := os.WriteFile(filepath.Join(root, "fake-pi"), []byte(shardFakeReviewer), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "shard-mode"), []byte(mode), 0600); err != nil {
		t.Fatal(err)
	}
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	v = mutate(t, bin, repo, home, v, "apply", "fixture-model", settings)
	b, _ := json.Marshal(map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": "none", "action_id": "prepare", "intent": "Add three features: alpha, beta and gamma", "target": target})
	v = axi(t, bin, repo, home, string(b))
	v = mutate(t, bin, repo, home, v, "resolve-rules", "rules", nil)
	if got := mutate(t, bin, repo, home, v, "review", "review", nil); got["outcome"] != "ok" {
		t.Fatalf("review refused: %v", got)
	}
	run := runOf(t, waitReview(t, bin, repo, home))
	calls := strings.Split(strings.TrimSpace(string(mustRead(t, filepath.Join(root, "shard-calls")))), "\n")
	slices.Sort(calls)
	return run, calls, home
}

func shardsOf(t *testing.T, run map[string]any) map[string]any {
	t.Helper()
	review, _ := run["review"].(map[string]any)
	shards, ok := review["shards"].(map[string]any)
	if !ok {
		t.Fatalf("review has no shard record: %v", review)
	}
	return shards
}

func shardStatuses(t *testing.T, shards map[string]any) map[string]string {
	t.Helper()
	statuses := map[string]string{}
	for _, item := range shards["shards"].([]any) {
		shard := item.(map[string]any)
		statuses[shard["name"].(string)] = shard["status"].(string)
	}
	if boundary, ok := shards["boundary"].(map[string]any); ok {
		statuses["boundary"] = boundary["status"].(string)
	}
	return statuses
}

func TestShardedReviewThroughDaemon(t *testing.T) {
	t.Parallel()
	wantCalls := []string{"boundary alpha/a1.txt,alpha/a2.txt,beta/b1.txt,beta/b2.txt,gamma/g1.txt", "plan", "shard:Alpha alpha/a1.txt,alpha/a2.txt", "shard:Beta beta/b1.txt,beta/b2.txt", "shard:Gamma gamma/g1.txt"}

	t.Run("all parts clean", func(t *testing.T) {
		t.Parallel()
		run, calls, _ := shardFixture(t, "clean")
		if !slices.Equal(calls, wantCalls) {
			t.Fatalf("calls = %q, want %q", calls, wantCalls)
		}
		if run["status"] != "clean-review" {
			t.Fatalf("status = %v, want clean-review: %v", run["status"], run)
		}
		shards := shardsOf(t, run)
		want := map[string]string{"Alpha": "clean-review", "Beta": "clean-review", "Gamma": "clean-review", "boundary": "clean-review"}
		if got := shardStatuses(t, shards); !maps.Equal(got, want) {
			t.Fatalf("shard statuses = %v, want %v", got, want)
		}
		if shards["planner"] != "planned" {
			t.Fatalf("planner = %v, want planned", shards["planner"])
		}
		result := run["review"].(map[string]any)["result"].(map[string]any)
		if result["verdict"] != "clean" || len(result["coverage"].([]any)) != 5 {
			t.Fatalf("aggregate result = %v", result)
		}
	})

	t.Run("one blocking shard", func(t *testing.T) {
		t.Parallel()
		run, calls, _ := shardFixture(t, "finding")
		want := append(slices.Clone(wantCalls), "confirm:shard:Beta beta/b1.txt,beta/b2.txt")
		slices.Sort(want)
		if !slices.Equal(calls, want) {
			t.Fatalf("calls = %q, want %q", calls, want)
		}
		if run["status"] != "outstanding-findings" {
			t.Fatalf("status = %v, want outstanding-findings", run["status"])
		}
		statuses := shardStatuses(t, shardsOf(t, run))
		wantStatuses := map[string]string{"Alpha": "clean-review", "Beta": "outstanding-findings", "Gamma": "clean-review", "boundary": "clean-review"}
		if !maps.Equal(statuses, wantStatuses) {
			t.Fatalf("shard statuses = %v, want %v", statuses, wantStatuses)
		}
		findings := run["review"].(map[string]any)["result"].(map[string]any)["findings"].([]any)
		if len(findings) != 1 || findings[0].(map[string]any)["file"] != "beta/b1.txt" {
			t.Fatalf("findings = %v, want the beta finding", findings)
		}
	})

	t.Run("invalid plan falls back to one shard", func(t *testing.T) {
		t.Parallel()
		run, calls, _ := shardFixture(t, "clean invalid")
		want := []string{"plan", "whole alpha/a1.txt,alpha/a2.txt,beta/b1.txt,beta/b2.txt,gamma/g1.txt"}
		if !slices.Equal(calls, want) {
			t.Fatalf("calls = %q, want %q", calls, want)
		}
		if run["status"] != "clean-review" {
			t.Fatalf("status = %v, want clean-review", run["status"])
		}
		shards := shardsOf(t, run)
		if planner, _ := shards["planner"].(string); !strings.Contains(planner, "alpha/a1.txt") {
			t.Fatalf("planner = %q, want the invalid-plan reason", planner)
		}
		if _, ok := shards["shards"]; ok {
			t.Fatalf("fallback recorded shard results: %v", shards)
		}
	})
}

func TestValidateShardPlan(t *testing.T) {
	t.Parallel()
	paths := []string{"a.go", "b.go", "c.go"}
	shard := func(name string, paths ...string) shardPlanEntry {
		return shardPlanEntry{Name: name, Summary: "s", Paths: paths}
	}
	tooMany := make([]shardPlanEntry, maxReviewShards+1)
	for i := range tooMany {
		tooMany[i] = shard(string(rune('A' + i)))
	}
	for _, test := range []struct {
		name string
		plan []shardPlanEntry
		want string
	}{
		{"valid", []shardPlanEntry{shard("A", "a.go", "b.go"), shard("B", "c.go")}, ""},
		{"empty", nil, "no shards"},
		{"over cap", tooMany, "exceed the cap"},
		{"unnamed", []shardPlanEntry{shard(" ", "a.go", "b.go", "c.go")}, "no name"},
		{"duplicate name", []shardPlanEntry{shard("A", "a.go"), shard("A", "b.go", "c.go")}, "used twice"},
		{"no paths", []shardPlanEntry{shard("A", "a.go", "b.go", "c.go"), shard("B")}, "no paths"},
		{"unknown path", []shardPlanEntry{shard("A", "a.go", "b.go", "c.go", "d.go")}, "not a changed path"},
		{"path in two shards", []shardPlanEntry{shard("A", "a.go", "b.go"), shard("B", "b.go", "c.go")}, "in shards"},
		{"missing path", []shardPlanEntry{shard("A", "a.go"), shard("B", "c.go")}, "b.go is in no shard"},
	} {
		err := validateShardPlan(test.plan, paths)
		if test.want == "" && err != nil || test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
			t.Errorf("%s: error = %v, want %q", test.name, err, test.want)
		}
	}
}

func TestAggregateShardReviewPrecedence(t *testing.T) {
	t.Parallel()
	clean := reviewOutcome{Status: "clean-review", Snapshot: "unchanged", Result: &Verdict{Verdict: "clean", Coverage: []string{"a.go"}, Findings: []Finding{}}}
	partial := reviewOutcome{Status: "partial-review", Snapshot: "unchanged", Result: &Verdict{Verdict: "partial", Coverage: []string{"b.go"}, Findings: []Finding{}, Uncovered: []UncoveredPath{{Path: "c.go", Reason: "review_budget"}}}}
	inconclusive := reviewOutcome{Status: "inconclusive", Snapshot: "unchanged", Answer: agentAnswer{Reason: "invalid_output"}}
	findings := reviewOutcome{Status: "outstanding-findings", Snapshot: "unchanged", Result: &Verdict{Verdict: "findings", Coverage: []string{"d.go"}, Findings: []Finding{{ID: "f", Priority: "P1", File: "d.go", Start: 1, End: 1}}}}
	paused := reviewOutcome{Status: "paused", Failure: &BackendFailure{Reason: "quota"}}
	for _, test := range []struct {
		name  string
		parts []reviewOutcome
		want  string
	}{
		{"all clean", []reviewOutcome{clean, clean}, "clean-review"},
		{"partial", []reviewOutcome{clean, partial}, "partial-review"},
		{"inconclusive over partial", []reviewOutcome{partial, inconclusive, clean}, "inconclusive"},
		{"findings over inconclusive", []reviewOutcome{inconclusive, findings, partial}, "outstanding-findings"},
		{"halted part", []reviewOutcome{findings, paused}, "paused"},
	} {
		got := aggregateShardReview(test.parts)
		if got.Status != test.want {
			t.Errorf("%s: status = %s, want %s", test.name, got.Status, test.want)
		}
		verdict := map[string]string{"clean-review": "clean", "partial-review": "partial", "outstanding-findings": "findings"}[got.Status]
		if verdict != "" && (got.Result == nil || got.Result.Verdict != verdict) {
			t.Errorf("%s: result %#v does not agree with status %s", test.name, got.Result, got.Status)
		}
	}
	got := aggregateShardReview([]reviewOutcome{clean, partial, findings})
	if !slices.Equal(got.Result.Coverage, []string{"a.go", "b.go", "d.go"}) || len(got.Result.Uncovered) != 1 || len(got.Result.Findings) != 1 {
		t.Fatalf("aggregate result = %#v", got.Result)
	}
}

func TestHostResultSummarizesShards(t *testing.T) {
	t.Parallel()
	r := &Run{Host: &Host{Name: "fake-host", ID: "task-7"}, Review: &Execution{ID: "review", Status: "outstanding-findings", Result: &Verdict{Verdict: "findings", Findings: []Finding{}}}}
	if got := hostResult(r, true); got.Shards != nil {
		t.Fatalf("unsharded review has a shard summary: %v", got.Shards)
	}
	r.Review.Shards = &ShardReview{Planner: "planned", Shards: []ReviewShard{
		{Name: "Alpha", Summary: "adds alpha", Paths: []string{"a.go"}, ShardResult: ShardResult{Status: "clean-review", Result: &Verdict{Verdict: "clean"}}},
		{Name: "Beta", Summary: "adds beta", Paths: []string{"b.go"}, ShardResult: ShardResult{Status: "outstanding-findings", Result: &Verdict{Verdict: "findings"}}},
	}, Boundary: &ShardResult{Status: "clean-review", Result: &Verdict{Verdict: "clean"}}}
	got := hostResult(r, true).Shards
	want := []HostShard{
		{Name: "Alpha", Summary: "adds alpha", Paths: []string{"a.go"}, Status: "clean-review", Verdict: "clean"},
		{Name: "Beta", Summary: "adds beta", Paths: []string{"b.go"}, Status: "outstanding-findings", Verdict: "findings"},
		{Name: "boundary", Boundary: true, Status: "clean-review", Verdict: "clean"},
	}
	if !slices.EqualFunc(got, want, func(a, b HostShard) bool {
		return a.Name == b.Name && a.Summary == b.Summary && slices.Equal(a.Paths, b.Paths) && a.Status == b.Status && a.Verdict == b.Verdict && a.Boundary == b.Boundary
	}) {
		t.Fatalf("host shards = %+v, want %+v", got, want)
	}
	if hostResult(r, true).Status != "outstanding-findings" {
		t.Fatal("shard summary changed the overall host status")
	}
}

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fullReviewer = `#!/usr/bin/python3
import json,sys,os
if sys.argv[1:] == ['--list-models']:
    print('provider model context')
    print('fixture model 128K')
    print('openai gpt-5.6-luna 128K')
    sys.exit(0)
root=os.path.dirname(__file__)
with open(root+'/review-invocations','a') as f: f.write('review\n')
b=json.load(sys.stdin)
with open(root+'/review-input.json','w') as f: json.dump(b,f,separators=(',',':'))
mode=open(root+'/review-mode').read().strip() if os.path.exists(root+'/review-mode') else 'clean'
v={'head':b['Head'],'verdict':'clean','coverage':b['Paths'],'findings':[],'documentation':{'status':'not-needed','reason':'no public behavior changed'}}
if mode=='docs-missing':
    v['documentation']={'status':'missing','reason':'README does not describe the new sample'}
if mode=='findings':
    v['verdict']='findings'
    v['findings']=[{'id':'x','priority':'P1','origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':'Sample is wrong','action':'Fix the sample','rule':'','head':b['Head']}]
if mode=='mixed':
    v['verdict']='findings'
    v['findings']=[{'id':'x','priority':p,'origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':e,'action':'Fix the sample','rule':'','head':b['Head']} for p,e in [('P1','Sample is wrong'),('P2','Sample is vague'),('P3','Sample could be shorter')]]
if b.get('Responses'):
    check=open(root+'/review-check').read().strip() if os.path.exists(root+'/review-check') else 'confirmed'
    v['resolutions']=[{'finding':r['finding'],'check':check,'reason':'checked against the candidate'} for r in b['Responses']]
provider,model=sys.argv[sys.argv.index('--model')+1].split('/',1)
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':provider,'model':model,'stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
`

const gateCommand = `#!/usr/bin/python3
import os,sys
root=os.path.dirname(__file__)
name=sys.argv[1]
with open(root+'/commands.log','a') as f: f.write(name+'\n')
if os.path.exists(root+'/sleep-'+name):
    import time
    time.sleep(float(open(root+'/sleep-'+name).read()))
if os.path.exists(root+'/kill-'+name):
    import signal
    os.kill(os.getppid(), signal.SIGKILL)
required=root+'/require-'+name
if os.path.exists(required):
    want=open(required).read()
    got=open('sample.txt').read()
    if got!=want:
        print('FAIL '+name+': sample.txt is '+repr(got))
        sys.exit(1)
print('ok '+name)
`

const gateFixer = `#!/usr/bin/python3
import json,sys,os
root=os.path.dirname(__file__)
b=json.load(sys.stdin)
with open(root+'/fix-invocations','a') as f: f.write(b['Stage']+'\n')
with open(root+'/fix-input.json','w') as f: json.dump(b,f)
def staged(name):
    for candidate in [root+'/fix-'+name+'-'+b['Stage'], root+'/fix-'+name]:
        if os.path.exists(candidate):
            return candidate
target=open(staged('content')).read() if staged('content') else 'repaired\n'
path=open(root+'/fix-path').read() if os.path.exists(root+'/fix-path') else 'sample.txt'
if os.path.exists(root+'/fix-fail'):
    sys.exit(1)
if staged('then-review'):
    open(root+'/review-mode','w').write(open(staged('then-review')).read())
if os.path.exists(root+'/fix-escalate'):
    answer=json.dumps({'escalate':open(root+'/fix-escalate').read(),'reason':'the correct repair changes the requested behavior'})
    print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':answer}]}}))
    print(json.dumps({'type':'agent_settled','aborted':False}))
    sys.exit(0)
response=open(root+'/fix-response').read().strip() if os.path.exists(root+'/fix-response') else 'fixed'
def respond():
    if not b.get('Selected') or response=='missing':
        return
    items=[]
    for f in b['Selected']:
        item={'finding':f['id'],'response':response,'reason':'answered by the fixture fixer'}
        if response=='deferred':
            item['authority']=b['Authorities'][0]['id'] if b.get('Authorities') else 'none'
        items.append(item)
    print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps({'responses':items})}]}}))
    print(json.dumps({'type':'agent_settled','aborted':False}))
if b.get('Selected') and response in ('disputed','deferred'):
    respond()
    sys.exit(0)
if os.path.exists(root+'/fix-extra'):
    extra=open(root+'/fix-extra').read()
    os.makedirs(os.path.dirname(extra) or '.', exist_ok=True)
    with open(extra,'w') as f: f.write('added\n')
if os.path.exists(root+'/fix-remove'):
    os.remove(open(root+'/fix-remove').read())
if target=='DELETE':
    os.remove(path)
elif target=='APPEND':
    with open(path,'a') as f: f.write('x\n')
else:
    with open(path,'w') as f: f.write(target)
respond()
`

func countLines(t *testing.T, path string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, line := range strings.Fields(string(b)) {
		counts[line]++
	}
	return counts
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
}

func writePrivateJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeConfigBytes(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeConfigBytes(path string, data []byte, perm os.FileMode) error {
	if has(configFiles, filepath.Base(path)) && !strings.Contains(string(data), `"version"`) {
		head := `{"version":2,`
		if filepath.Base(path) == "pipeline.json" {
			head += `"budget_unit":"rounds",`
		}
		body := strings.TrimPrefix(strings.TrimSpace(string(data)), "{")
		if strings.TrimSpace(body) == "}" {
			head = strings.TrimSuffix(head, ",")
		}
		data = []byte(head + body)
	}
	if filepath.Base(path) == "backends.json" && !strings.Contains(string(data), `"writable"`) {
		root, _ := json.Marshal(filepath.Dir(filepath.Dir(path)))
		data = []byte(strings.Replace(string(data), "{", `{"writable":{"reviewer":[`+string(root)+`],"fixer":[`+string(root)+`]},`, 1))
		data = []byte(strings.Replace(string(data), ",}", "}", 1))
	}
	return os.WriteFile(path, data, perm)
}

// allowFixtureWrites lets fake agents record invocations beside the state
// directory, which OS confinement would otherwise deny.
func allowFixtureWrites(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, "backends.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.MkdirAll(home, 0700)
		if err = writeConfigBytes(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func installPipelineProvider(t *testing.T, home string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "model"}, {"id": "gpt-5.6-luna"}}})
	}))
	t.Cleanup(server.Close)
	allowFixtureWrites(t, home)
	writePrivateJSON(t, filepath.Join(home, "providers.json"), TrustedProviderConfig{Providers: map[string]ProviderConfig{"fixture": {BaseURL: server.URL, APIKeyEnv: "GREENHOUSE_TEST_PROVIDER_KEY", API: "openai-completions"}, "openai": {BaseURL: server.URL, APIKeyEnv: "GREENHOUSE_TEST_PROVIDER_KEY", API: "openai-completions"}}})
}

func pipelineFixture(t *testing.T, plan func(root string) map[string]any) (string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home := fixture(t)
	root := filepath.Dir(home)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Sample\n"), 0600)
	git(t, repo, "add", "sample.txt", "README.md")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(root, "fake-pi"), fullReviewer)
	writeExecutable(t, filepath.Join(root, "gate"), gateCommand)
	writeExecutable(t, filepath.Join(root, "fake-fixer"), gateFixer)
	writePrivateJSON(t, filepath.Join(home, "reviewer.json"), ReviewerConfig{Executable: filepath.Join(root, "fake-pi"), Model: "fixture/model"})
	writePrivateJSON(t, filepath.Join(home, "fixer.json"), map[string]any{"executable": filepath.Join(root, "fake-fixer"), "model": "fixture/model", "name": "Test", "email": "test@example.invalid", "checks": [][]string{{"/usr/bin/true"}}})
	installPipelineProvider(t, home)
	writePrivateJSON(t, filepath.Join(home, "pipeline.json"), plan(root))
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	v = mutate(t, bin, repo, home, v, "apply", "full", map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": v["gate_id"], "action_id": "prepare", "intent": "Change the sample", "target": target}
	b, _ := json.Marshal(q)
	v = axi(t, bin, repo, home, string(b))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	return bin, repo, home, v
}

func gateArgv(root, name string) []string { return []string{filepath.Join(root, "gate"), name} }

func waitPipeline(t *testing.T, bin, repo, home string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		v := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		if s := runOf(t, v)["status"]; s != "running" && s != "cancelling" {
			return v
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("pipeline did not settle")
	return nil
}

func stageOf(t *testing.T, v map[string]any, name string) map[string]any {
	t.Helper()
	p, ok := runOf(t, v)["pipeline"].(map[string]any)
	if !ok {
		t.Fatalf("no pipeline: %v", v)
	}
	for _, s := range p["stages"].([]any) {
		if stage := s.(map[string]any); stage["name"] == name {
			return stage
		}
	}
	t.Fatalf("no stage %s: %v", name, p)
	return nil
}

func TestFullPipelineCommandStagesDispatchNoAgent(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture repository publishes no documentation"}}
	})
	root := filepath.Dir(home)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	if run["status"] != "needs-decision" || stageOf(t, done, "push")["status"] != "unconfigured" {
		t.Fatalf("pipeline passed without trusted delivery settings: %v", run["pipeline"])
	}
	want := map[string]string{"intent": "passed", "sync": "passed", "review": "passed", "test": "passed", "docs": "not-applicable", "lint": "passed", "push": "unconfigured", "pr": "pending", "ci": "pending"}
	for name, status := range want {
		if got := stageOf(t, done, name)["status"]; got != status {
			t.Fatalf("stage %s = %v, want %s", name, got, status)
		}
	}
	if reason := stageOf(t, done, "docs")["reason"]; reason != "fixture repository publishes no documentation" {
		t.Fatalf("not-applicable reason lost: %v", reason)
	}
	test := stageOf(t, done, "test")
	commands := test["commands"].([]any)
	if len(commands) != 1 {
		t.Fatal(test)
	}
	command := commands[0].(map[string]any)
	if command["exit"] != float64(0) || command["executable_digest"] == "" || !strings.Contains(command["output"].(string), "ok test") || test["candidate"] != run["candidate"] || test["inputs"] == "" {
		t.Fatalf("command evidence incomplete: %v", test)
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 1 {
		t.Fatalf("review invocations %v", got)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); len(got) != 0 {
		t.Fatalf("command stages dispatched a fixer: %v", got)
	}
	if got := countLines(t, filepath.Join(root, "commands.log")); got["test"] != 1 || got["lint"] != 1 {
		t.Fatalf("configured commands did not each run once: %v", got)
	}
	if run["review_used"] != float64(1) || run["pipeline"].(map[string]any)["repair_used"] != float64(0) {
		t.Fatal(run)
	}
}

func TestMissingCommandBlocksAsUnconfigured(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "not_applicable": map[string]string{"docs": "no documentation"}}
	})
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	lint := stageOf(t, done, "lint")
	if run["status"] != "needs-decision" || lint["status"] != "unconfigured" || run["pipeline"].(map[string]any)["failed"] != "lint" {
		t.Fatalf("missing lint command did not block: %v", run["pipeline"])
	}
	blocked := strings.Join(func() []string {
		var out []string
		for _, b := range done["blocked"].([]any) {
			out = append(out, b.(string))
		}
		return out
	}(), "\n")
	if !strings.Contains(blocked, "pipeline stopped at lint") {
		t.Fatal(blocked)
	}
	again := mutate(t, bin, repo, home, done, "pipeline", "continue", nil)
	if again["outcome"] != "ok" {
		t.Fatal(again)
	}
	again = waitPipeline(t, bin, repo, home)
	if runOf(t, again)["status"] != "needs-decision" || stageOf(t, again, "lint")["status"] != "unconfigured" {
		t.Fatal(again)
	}
	root := filepath.Dir(home)
	if got := countLines(t, filepath.Join(root, "commands.log")); got["test"] != 1 {
		t.Fatalf("continue reran current evidence: %v", got)
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 1 {
		t.Fatalf("continue reran review: %v", got)
	}
}

func TestPassingDocBuildDoesNotProveDocumentation(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "docs": [][]string{gateArgv(root, "docs")}, "lint": [][]string{gateArgv(root, "lint")}}
	})
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("docs-missing"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	docs := stageOf(t, done, "docs")
	if runOf(t, done)["status"] != "needs-decision" || docs["status"] != "failed" || !strings.Contains(docs["reason"].(string), "README does not describe the new sample") {
		t.Fatalf("doc impact not enforced: %v", docs)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); len(got) != 0 {
		t.Fatalf("unauthorized docs repair dispatched: %v", got)
	}
	if got := countLines(t, filepath.Join(root, "commands.log")); got["lint"] != 0 {
		t.Fatalf("pipeline advanced past failed docs: %v %v", got, runOf(t, done)["pipeline"])
	}
}

func repairPlan(root string) map[string]any {
	return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "no documentation"}, "repairs": []string{"test"}, "repair_limit": 2}
}

func TestGateRepairRerunsReviewAndGates(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	submitted := runOf(t, v)["submitted"].(map[string]any)["head"]
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	p := run["pipeline"].(map[string]any)
	if run["status"] != "needs-decision" || stageOf(t, done, "push")["status"] != "unconfigured" {
		t.Fatalf("repair did not stop at unconfigured delivery: %v", p)
	}
	if run["candidate"] == submitted || p["repair_used"] != float64(1) || run["review_used"] != float64(2) || run["repair_used"] != float64(0) {
		t.Fatalf("budgets or candidate identity wrong: %v", run)
	}
	for _, name := range []string{"review", "test", "lint"} {
		if st := stageOf(t, done, name); st["status"] != "passed" || st["candidate"] != run["candidate"] {
			t.Fatalf("stage %s evidence not rerun for repaired candidate: %v", name, st)
		}
	}
	if st := stageOf(t, done, "test"); st["repairs"] != float64(1) || st["runs"] != float64(2) {
		t.Fatal(st)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["test"] != 1 {
		t.Fatalf("fixer invocations %v", got)
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 2 {
		t.Fatalf("review not rerun after repair: %v", got)
	}
	if got := countLines(t, filepath.Join(root, "commands.log")); got["test"] != 2 || got["lint"] != 1 {
		t.Fatalf("gates not rerun exactly once after repair: %v", got)
	}
	reviews := p["reviews"].([]any)
	if len(reviews) != 1 || reviews[0].(map[string]any)["rules"].(map[string]any)["candidate"] != submitted {
		t.Fatalf("superseded review evidence not retained: %v", reviews)
	}
	repair := p["repairs"].([]any)[0].(map[string]any)
	if repair["stage"] != "test" || repair["base"] != submitted || repair["head"] != run["candidate"] {
		t.Fatal(repair)
	}
	if head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")); head != submitted {
		t.Fatalf("source moved before handback: %s", head)
	}
	if mutate(t, bin, repo, home, done, "new", "new-before-handback", nil)["outcome"] != "refused" {
		t.Fatal("unpublished pipeline custody allowed New run")
	}
	os.WriteFile(filepath.Join(repo, "dirty"), []byte("preserved"), 0600)
	refused := mutate(t, bin, repo, home, done, "handback", "dirty-handback", nil)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "CI-ready") {
		t.Fatal(refused)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "dirty")); string(b) != "preserved" {
		t.Fatal("dirty source work changed")
	}
	if strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")) != submitted {
		t.Fatal("blocked delivery changed the source branch")
	}
}

func TestGateRepairFailuresAreBounded(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	os.WriteFile(filepath.Join(root, "fix-content"), []byte("APPEND"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	test := stageOf(t, done, "test")
	if run["status"] != "needs-decision" || test["status"] != "failed" || !strings.Contains(test["reason"].(string), "gate repair budget exhausted (2/2)") {
		t.Fatalf("failure not bounded: %v", run["pipeline"])
	}
	if !strings.Contains(test["commands"].([]any)[0].(map[string]any)["output"].(string), "FAIL test") {
		t.Fatalf("diagnostics missing at failed stage: %v", test)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["test"] != 2 {
		t.Fatalf("fixer invocations %v", got)
	}
	if got := countLines(t, filepath.Join(root, "commands.log")); got["test"] != 3 || got["lint"] != 0 {
		t.Fatalf("commands %v", got)
	}
	again := mutate(t, bin, repo, home, done, "pipeline", "continue", nil)
	again = waitPipeline(t, bin, repo, home)
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["test"] != 2 || runOf(t, again)["pipeline"].(map[string]any)["repair_used"] != float64(2) {
		t.Fatalf("continue exceeded repair budget: %v", got)
	}
}

func TestRepairedHeadStaysUnreviewedUntilReviewPasses(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("findings"), 0600)
	v = mutate(t, bin, repo, home, v, "apply", "no-room-for-finding-repair", map[string]any{"mode": "auto", "preset": "full", "limit": 3, "rules": []string{"unslop"}})
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	if run["status"] != "needs-decision" || stageOf(t, done, "review")["status"] != "needs-decision" {
		t.Fatalf("review findings on repaired head did not park: %v", run["pipeline"])
	}
	if st := stageOf(t, done, "test"); st["status"] != "repaired-unreviewed" {
		t.Fatalf("repaired stage shown as %v", st["status"])
	}
	if len(run["findings"].([]any)) != 1 {
		t.Fatal(run["findings"])
	}
}

func TestRestartPreservesStageStateWithoutDuplicateWork(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	os.WriteFile(filepath.Join(root, "fix-content"), []byte("APPEND"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	before := countLines(t, filepath.Join(root, "commands.log"))
	stopDaemon(t, home)
	reattached := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	a, _ := json.Marshal(runOf(t, parked)["pipeline"])
	b, _ := json.Marshal(runOf(t, reattached)["pipeline"])
	if string(a) != string(b) || runOf(t, reattached)["review_used"] != runOf(t, parked)["review_used"] || reattached["token"] != parked["token"] {
		t.Fatalf("restart changed parked pipeline state:\n%s\n%s", a, b)
	}
	time.Sleep(300 * time.Millisecond)
	if after := countLines(t, filepath.Join(root, "commands.log")); after["test"] != before["test"] || after["lint"] != before["lint"] {
		t.Fatalf("reattach executed work: %v %v", before, after)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["test"] != 2 {
		t.Fatalf("reattach dispatched a fixer: %v", got)
	}
}

func TestRestartDuringCommandParksWithoutRetry(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "sleep-test"), []byte("1.5"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && countLines(t, filepath.Join(root, "commands.log"))["test"] == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	killRepairDaemon(t, home)
	time.Sleep(2 * time.Second)
	recovered := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	run := runOf(t, recovered)
	if run["status"] != "recovery-required" || stageOf(t, recovered, "test")["status"] != "unknown-outcome" || run["review_used"] != float64(1) {
		t.Fatalf("interrupted command not parked: %v", run["pipeline"])
	}
	for _, action := range recovered["actions"].([]any) {
		if action == "pipeline" || action == "new" {
			t.Fatalf("unsafe action %v advertised after interruption", action)
		}
	}
	time.Sleep(300 * time.Millisecond)
	if got := countLines(t, filepath.Join(root, "commands.log")); got["test"] != 1 || got["lint"] != 0 {
		t.Fatalf("restart retried work: %v", got)
	}
}

func conflictFixture(t *testing.T, env ...string) (string, string, string, string, string, string, map[string]any) {
	t.Helper()
	bin, repo, home := fixture(t)
	root := filepath.Dir(home)
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "checkout", "-q", "-b", "target")
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("target side\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "target")
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "checkout", "-q", "main")
	if strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")) != base {
		t.Fatal("fixture branch")
	}
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	submitted := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	os.MkdirAll(home, 0700)
	writeExecutable(t, filepath.Join(root, "fake-pi"), fullReviewer)
	writeExecutable(t, filepath.Join(root, "gate"), gateCommand)
	writeExecutable(t, filepath.Join(root, "fake-fixer"), gateFixer)
	writePrivateJSON(t, filepath.Join(home, "reviewer.json"), ReviewerConfig{Executable: filepath.Join(root, "fake-pi"), Model: "fixture/model"})
	writePrivateJSON(t, filepath.Join(home, "fixer.json"), map[string]any{"executable": filepath.Join(root, "fake-fixer"), "model": "fixture/model", "name": "Test", "email": "test@example.invalid", "checks": [][]string{{"/usr/bin/true"}}})
	installPipelineProvider(t, home)
	writePrivateJSON(t, filepath.Join(home, "pipeline.json"), repairPlan(root))
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`, env...)
	v = mutate(t, bin, repo, home, v, "apply", "full", map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": v["gate_id"], "action_id": "prepare", "intent": "Change the sample", "target": target}
	b, _ := json.Marshal(q)
	v = axi(t, bin, repo, home, string(b))
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	return bin, repo, home, root, target, submitted, parked
}

func TestTargetConflictBecomesGatedFixerAction(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, target, submitted, parked := conflictFixture(t)
	run := runOf(t, parked)
	p := run["pipeline"].(map[string]any)
	if run["status"] != "needs-decision" || stageOf(t, parked, "sync")["status"] != "needs-decision" || parked["gate_id"] == "none" {
		t.Fatalf("conflict not gated: %v", p)
	}
	if conflict := p["conflict"].([]any); len(conflict) != 1 || conflict[0] != "sample.txt" {
		t.Fatal(conflict)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); len(got) != 0 {
		t.Fatalf("conflict dispatched a fixer before authorization: %v", got)
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); len(got) != 0 {
		t.Fatalf("review ran against an unsynchronized candidate: %v", got)
	}
	stale := map[string]any{}
	for k, val := range parked {
		stale[k] = val
	}
	stale["gate_id"] = "none"
	if mutate(t, bin, repo, home, stale, "resolve-conflict", "stale-gate", nil)["outcome"] != "refused" {
		t.Fatal("conflict resolution accepted without the gate identity")
	}
	resolved := mutate(t, bin, repo, home, parked, "resolve-conflict", "resolve", nil)
	if resolved["outcome"] != "ok" {
		t.Fatal(resolved)
	}
	done := waitPipeline(t, bin, repo, home)
	run = runOf(t, done)
	if run["status"] != "needs-decision" || stageOf(t, done, "push")["status"] != "unconfigured" || run["pipeline"].(map[string]any)["repair_used"] != float64(1) {
		t.Fatalf("resolved conflict did not converge: %v", run["pipeline"])
	}
	worktree := run["pipeline"].(map[string]any)["worktree"].(string)
	parents := strings.Fields(git(t, worktree, "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parents) != 3 || parents[0] != run["candidate"] || parents[1] != submitted || parents[2] != target {
		t.Fatalf("target sync rewrote history instead of merging: %v", parents)
	}
	if head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")); head != submitted {
		t.Fatalf("source changed without handback: %s", head)
	}
}

func TestChangedInputsInvalidateOnlyAffectedEvidence(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	ready := waitPipeline(t, bin, repo, home)
	if runOf(t, ready)["status"] != "needs-decision" {
		t.Fatal(runOf(t, ready)["pipeline"])
	}
	raised := mutate(t, bin, repo, home, ready, "apply", "raise-limit", map[string]any{"mode": "ask", "preset": "full", "limit": 7, "rules": []string{"unslop"}})
	if raised["outcome"] != "ok" || stageOf(t, raised, "review")["status"] != "passed" || runOf(t, raised)["pipeline"].(map[string]any)["status"] == "ci-ready" {
		t.Fatalf("a mode or limit change invalidated review evidence: %v", runOf(t, raised)["pipeline"])
	}
	changed := mutate(t, bin, repo, home, raised, "apply", "change-rules", map[string]any{"mode": "auto", "preset": "full", "limit": 7, "rules": []string{"unslop", "no-comments"}})
	if changed["outcome"] != "ok" || runOf(t, changed)["status"] == "ci-ready" || runOf(t, changed)["pipeline"].(map[string]any)["status"] == "ci-ready" {
		t.Fatalf("configuration change kept CI readiness: %v", runOf(t, changed)["pipeline"])
	}
	if stageOf(t, changed, "review")["status"] != "stale" || stageOf(t, changed, "test")["status"] != "passed" {
		t.Fatalf("invalidation not limited to declared inputs: %v", runOf(t, changed)["pipeline"])
	}
	mutate(t, bin, repo, home, changed, "pipeline", "continue", nil)
	done := waitPipeline(t, bin, repo, home)
	if runOf(t, done)["status"] != "needs-decision" {
		t.Fatal(runOf(t, done)["pipeline"])
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 2 {
		t.Fatalf("changed configuration did not rerun review: %v", got)
	}
	if got := countLines(t, filepath.Join(root, "commands.log")); got["test"] != 1 || got["lint"] != 1 {
		t.Fatalf("unaffected gates reran: %v", got)
	}
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "moved")
	moved := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	for _, action := range moved["actions"].([]any) {
		if action == "pipeline" || action == "handback" {
			t.Fatalf("moved source still offers %v", action)
		}
	}
}

func TestPipelineCancellationStopsCommandAndKeepsBudget(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "sleep-test"), []byte("30"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && countLines(t, filepath.Join(root, "commands.log"))["test"] == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	current := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	if cancelled := mutate(t, bin, repo, home, current, "cancel", "cancel", nil); cancelled["outcome"] != "ok" {
		t.Fatal(cancelled)
	}
	done := waitPipeline(t, bin, repo, home)
	if runOf(t, done)["status"] != "cancelled" || stageOf(t, done, "test")["status"] != "cancelled" || runOf(t, done)["review_used"] != float64(1) {
		t.Fatalf("cancellation not recorded: %v", runOf(t, done)["pipeline"])
	}
	os.Remove(filepath.Join(root, "sleep-test"))
	mutate(t, bin, repo, home, done, "pipeline", "continue", nil)
	if runOf(t, waitPipeline(t, bin, repo, home))["status"] != "needs-decision" {
		t.Fatal("explicit continue after cancellation did not finish")
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 1 {
		t.Fatalf("continue reran current review: %v", got)
	}
}

func TestUnknownCommandOutcomeHoldsRecoveryCustody(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "kill-test"), []byte("1"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	if run["status"] != "recovery-required" || stageOf(t, done, "test")["status"] != "unknown-outcome" {
		t.Fatalf("unknown command outcome did not require recovery: %v", run["pipeline"])
	}
	for _, action := range done["actions"].([]any) {
		if action == "pipeline" || action == "new" {
			t.Fatalf("unknown outcome still offers %v", action)
		}
	}
	if mutate(t, bin, repo, home, done, "new", "new-after-unknown", nil)["outcome"] != "refused" {
		t.Fatal("unknown command outcome allowed New run")
	}
	stopDaemon(t, home)
	if runOf(t, axi(t, bin, repo, home, `{"version":1,"op":"attach"}`))["status"] != "recovery-required" {
		t.Fatal("restart dropped the recovery hazard")
	}
	linked := filepath.Join(filepath.Dir(repo), "linked")
	git(t, repo, "worktree", "add", "-q", "-b", "linked", linked, "HEAD")
	other := axi(t, bin, linked, home, `{"version":1,"op":"attach"}`)
	other = mutate(t, bin, linked, home, other, "apply", "linked-full", map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}})
	q := map[string]any{"version": 1, "op": "prepare", "run_id": other["run_id"], "input_id": other["input_id"], "expected": other["token"], "gate_id": other["gate_id"], "action_id": "prepare-linked", "intent": "Change the sample", "target": run["target"]}
	b, _ := json.Marshal(q)
	other = axi(t, bin, linked, home, string(b))
	if other["outcome"] != "ok" {
		t.Fatal(other)
	}
	for _, action := range other["actions"].([]any) {
		if action == "pipeline" || action == "repair" {
			t.Fatalf("linked worktree advertises %v while custody is ambiguous", action)
		}
	}
	if !strings.Contains(strings.Join(func() []string {
		var out []string
		for _, b := range other["blocked"].([]any) {
			out = append(out, b.(string))
		}
		return out
	}(), "\n"), "custody") {
		t.Fatalf("linked worktree does not show the custody hazard: %v", other["blocked"])
	}
	if got := mutate(t, bin, linked, home, other, "pipeline", "linked-start", nil); got["outcome"] != "refused" || !strings.Contains(got["error"].(string), "custody") {
		t.Fatalf("linked worktree ignored ambiguous custody: %v", got)
	}
}

func TestRetainedFindingsBlockCleanFullReview(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	v = mutate(t, bin, repo, home, v, "apply", "ask", map[string]any{"mode": "ask", "preset": "full", "limit": 6, "rules": []string{"unslop"}})
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	if stageOf(t, parked, "review")["status"] != "needs-decision" {
		t.Fatal(runOf(t, parked)["pipeline"])
	}
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("clean"), 0600)
	changed := mutate(t, bin, repo, home, parked, "apply", "change-rules", map[string]any{"mode": "ask", "preset": "full", "limit": 7, "rules": []string{"unslop", "no-comments"}})
	mutate(t, bin, repo, home, changed, "pipeline", "continue", nil)
	done := waitPipeline(t, bin, repo, home)
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 2 {
		t.Fatalf("review not rerun: %v", got)
	}
	if runOf(t, done)["status"] == "ci-ready" || stageOf(t, done, "review")["status"] != "needs-decision" || len(runOf(t, done)["findings"].([]any)) != 1 {
		t.Fatalf("omitted retained finding was treated as resolved: %v", runOf(t, done)["pipeline"])
	}
	if got := countLines(t, filepath.Join(root, "commands.log")); got["test"] != 0 {
		t.Fatalf("gates ran past unresolved findings: %v", got)
	}
}

func TestNotApplicableDocsStillRequiresDocumentation(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := pipelineFixture(t, repairPlan)
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("docs-missing"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	docs := stageOf(t, done, "docs")
	if runOf(t, done)["status"] == "locally-ready" || docs["status"] != "failed" || !strings.Contains(docs["reason"].(string), "README does not describe the new sample") {
		t.Fatalf("not-applicable doc build hid missing documentation: %v", docs)
	}
}

func gitWrapper(t *testing.T, root string) string {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "wrapped-bin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(dir, "git"), `#!/bin/sh
case " $* " in
*" merge --no-ff "*) [ -e "`+root+`/kill-merge" ] && { "`+real+`" "$@"; kill -KILL $PPID; exit 0; } ;;
*" merge --abort "*) [ -e "`+root+`/kill-abort" ] && { "`+real+`" "$@"; kill -KILL $PPID; exit 0; } ;;
esac
exec "`+real+`" "$@"
`)
	return "PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func assertRecoveryCustody(t *testing.T, bin, repo, home string, v map[string]any, path string) {
	t.Helper()
	run := runOf(t, v)
	if run["status"] != "recovery-required" || run["pipeline"].(map[string]any)["status"] != "recovery-required" {
		t.Fatalf("unknown Git outcome did not require recovery: %v", run["pipeline"])
	}
	repairs := run["pipeline"].(map[string]any)["repairs"].([]any)
	if last := repairs[len(repairs)-1].(map[string]any); last["status"] != "unknown-outcome" || last["writer"] != "unknown" {
		t.Fatalf("repair record lost the unknown outcome: %v", last)
	}
	for _, action := range v["actions"].([]any) {
		if action == "pipeline" || action == "new" || action == "resolve-conflict" {
			t.Fatalf("unknown outcome still offers %v", action)
		}
	}
	if mutate(t, bin, repo, home, v, "new", "new-after-unknown-git", nil)["outcome"] != "refused" {
		t.Fatal("unknown Git outcome allowed New run")
	}
	stopDaemon(t, home)
	if got := runOf(t, axi(t, bin, repo, home, `{"version":1,"op":"attach"}`, path)); got["status"] != "recovery-required" {
		t.Fatalf("restart dropped recovery custody: %v", got["status"])
	}
}

func TestConflictMergeUnknownOutcomeLaunchesNothing(t *testing.T) {
	t.Parallel()
	scratch := t.TempDir()
	path := gitWrapper(t, scratch)
	bin, repo, home, root, _, _, parked := conflictFixture(t, path)
	os.WriteFile(filepath.Join(scratch, "kill-merge"), []byte("1"), 0600)
	if resolved := mutate(t, bin, repo, home, parked, "resolve-conflict", "resolve", nil); resolved["outcome"] != "ok" {
		t.Fatal(resolved)
	}
	done := waitPipeline(t, bin, repo, home)
	if got := countLines(t, filepath.Join(root, "fix-invocations")); len(got) != 0 {
		t.Fatalf("fixer ran after an unknown merge outcome: %v", got)
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); len(got) != 0 {
		t.Fatalf("review ran after an unknown merge outcome: %v", got)
	}
	assertRecoveryCustody(t, bin, repo, home, done, path)
}

func TestConflictAbortUnknownOutcomeHoldsCustody(t *testing.T) {
	t.Parallel()
	scratch := t.TempDir()
	path := gitWrapper(t, scratch)
	bin, repo, home, root, _, _, parked := conflictFixture(t, path)
	os.WriteFile(filepath.Join(root, "fix-fail"), []byte("1"), 0600)
	os.WriteFile(filepath.Join(scratch, "kill-abort"), []byte("1"), 0600)
	if resolved := mutate(t, bin, repo, home, parked, "resolve-conflict", "resolve", nil); resolved["outcome"] != "ok" {
		t.Fatal(resolved)
	}
	done := waitPipeline(t, bin, repo, home)
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["sync"] != 1 {
		t.Fatalf("fixer invocations %v", got)
	}
	assertRecoveryCustody(t, bin, repo, home, done, path)
}

func TestFullPipelineRereviewsRepairThatRestoresTargetTree(t *testing.T) {
	bin, repo, home, v := pipelineFixture(t, localPlan)
	root := filepath.Dir(home)
	target := runOf(t, v)["target"].(string)
	git(t, repo, "checkout", "--detach", target)
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("original\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "baseline sample")
	target = strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "checkout", "-b", "restoration")
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("regression\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "regression")
	v = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	v = mutate(t, bin, repo, home, v, "new", "restoration-new", nil)
	v = mutate(t, bin, repo, home, v, "apply", "restoration-full", Settings{Mode: "auto", Preset: "full", Limit: 6, Rules: []string{"unslop"}, ReviewerBackend: "pi", ReviewerModel: "fixture/model", FixerBackend: "pi", FixerModel: "fixture/model"})
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "gate_id": v["gate_id"], "expected": v["token"], "action_id": "restoration-prepare", "intent": "Preserve original sample", "target": target}
	b, _ := json.Marshal(q)
	v = axi(t, bin, repo, home, string(b))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	os.WriteFile(filepath.Join(root, "review-mode"), []byte("findings"), 0600)
	os.WriteFile(filepath.Join(root, "fix-content"), []byte("original\n"), 0600)
	os.WriteFile(filepath.Join(root, "fix-then-review"), []byte("clean"), 0600)
	v = mutate(t, bin, repo, home, v, "pipeline", "restore-tree", nil)
	v = waitPipeline(t, bin, repo, home)
	if runOf(t, v)["status"] != "locally-ready" {
		t.Fatalf("restored target tree did not get independent rereview and gates: %v", v)
	}
	if n := countLines(t, filepath.Join(root, "review-invocations"))["review"]; n != 2 {
		t.Fatalf("want two independent reviews, got %d", n)
	}
}

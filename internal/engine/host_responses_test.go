package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailedHostReviewAdmissionDoesNotChangeFindingHistory(t *testing.T) {
	finding := Finding{ID: "finding-1", Priority: "P1", Origin: "correctness", File: "sample.txt", Start: 1, End: 1, Explanation: "unsafe", Action: "fix it", Head: "candidate"}
	previousResponses := &Repair{ID: "previous"}
	r := &Run{
		Host:          &Host{Name: "host", ID: "task"},
		Review:        &Execution{ID: "prior-review", Grant: "prior-grant", Status: "outstanding-findings", Result: &Verdict{Findings: []Finding{finding}}},
		HostFindings:  []Finding{finding},
		HostHistory:   []PriorFinding{{Finding: finding, State: "open"}},
		HostResponses: previousResponses,
	}
	if err := admitHostReview(r, "next-grant", []ResolutionResponse{{Finding: finding.ID, Response: "disputed", Reason: "not a defect"}}); err == nil {
		t.Fatal("review admission without intent and target succeeded")
	}
	if r.HostHistory[0].State != "open" {
		t.Fatalf("failed admission changed prior finding state: %+v", r.HostHistory[0])
	}
	if r.HostResponses != previousResponses || len(r.HostGrants) != 0 {
		t.Fatalf("failed admission changed host response state: responses=%+v grants=%v", r.HostResponses, r.HostGrants)
	}
}

func TestHostReviewRefusalDoesNotRecordResponse(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("trust = os.Getenv(\"APP_MODE\")\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	installFake(t, home, "")
	v := hostPrepare(t, bin, repo, home, axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"fake-host","id":"task-7"}}`), target, "-refusal")
	v = hostCall(t, bin, repo, home, v, "review", "initial-review", map[string]any{"grant": "initial-review"})
	if v["outcome"] != "ok" {
		t.Fatalf("initial review: %v", v)
	}
	v = hostSettle(t, bin, repo, home)
	findings, _ := v["host_result"].(map[string]any)["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("initial review did not report the expected finding: %v", v)
	}
	finding := findings[0].(map[string]any)
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 1, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/changed-model", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	v = hostCall(t, bin, repo, home, v, "apply", "change-reviewer-settings", map[string]any{"settings": settings})
	if v["outcome"] != "ok" {
		t.Fatalf("change reviewer settings: %v", v)
	}
	v = hostCall(t, bin, repo, home, v, "review", "refused-review", map[string]any{"grant": "refused-review", "responses": []map[string]any{{"finding": finding["id"], "response": "disputed", "reason": "not a defect"}}})
	if v["outcome"] != "refused" {
		t.Fatalf("review should be refused until rules are resolved: %v", v)
	}
	history, _ := runOf(t, v)["host_history"].([]any)
	if len(history) != 1 || history[0].(map[string]any)["state"] != "open" {
		t.Fatalf("refused review changed finding history: %v", history)
	}
}

func TestHostResponsesAcrossThreeRounds(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("first defect\nsecond defect\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	installFake(t, home, `
responses=b.get('Responses',[])
countfile=root+'/host-round'
roundno=int(open(countfile).read())+1 if os.path.exists(countfile) else 1
with open(countfile,'w') as f: f.write(str(roundno))
v={'head':b['Head'],'verdict':'findings','coverage':b['Paths'],'findings':[]}
if responses:
 assert len(responses)==2
 assert [r['response'] for r in responses]==['fixed','disputed']
 assert all(r['prior']['id']==r['finding'] for r in responses)
 assert responses[1]['reason']=='The behavior is intentional'
 vcheck='rejected' if roundno==2 else 'confirmed'
 v['resolutions']=[{'finding':r['finding'],'check':vcheck,'reason':'Independently checked '+r['response']} for r in responses]
 if roundno==2:
  v['findings']=[dict(r['prior'],head=b['Head']) for r in responses]
 else:
  v['verdict']='clean'
else:
 for line in [1,2]:
  v['findings'].append({'id':'ignored','priority':'P2','origin':'correctness','file':'sample.txt','start':line,'end':line,'explanation':'Defect '+str(line),'action':'Fix line '+str(line),'rule':'','head':b['Head']})
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
sys.exit(0)
`)
	b, _ := json.Marshal(map[string]any{"version": 2, "op": "attach", "host": fakeHost})
	v := hostPrepare(t, bin, repo, home, axi(t, bin, repo, home, string(b)), target, "-initial")
	v = hostCall(t, bin, repo, home, v, "review", "round-1", map[string]any{"grant": "round-1"})
	if v["outcome"] != "ok" {
		t.Fatalf("first round: %v", v)
	}
	v = hostSettleWithFindings(t, bin, repo, home)
	findings := v["host_result"].(map[string]any)["findings"].([]any)
	if len(findings) != 2 {
		t.Fatalf("findings: %v", findings)
	}
	responses := []map[string]any{
		{"finding": findings[0].(map[string]any)["id"], "response": "fixed"},
		{"finding": findings[1].(map[string]any)["id"], "response": "disputed", "reason": "The behavior is intentional"},
	}
	for round := 2; round <= 3; round++ {
		if round == 2 {
			if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("corrected first line\nsecond defect\n"), 0600); err != nil {
				t.Fatal(err)
			}
			git(t, repo, "-c", "user.name=Author", "-c", "user.email=author@example.invalid", "commit", "-am", "author repair")
			v = hostSettle(t, bin, repo, home)
			v = hostCall(t, bin, repo, home, v, "new", "successor", nil)
			v = hostPrepare(t, bin, repo, home, v, target, "-successor")
		}
		before := git(t, repo, "status", "--porcelain")
		refs := git(t, repo, "show-ref")
		if round == 2 {
			invalid := []struct {
				name      string
				responses any
			}{
				{"unknown", []map[string]any{{"finding": "unknown", "response": "fixed"}}},
				{"duplicate", []map[string]any{responses[0], responses[0]}},
				{"deferred", []map[string]any{{"finding": responses[0]["finding"], "response": "deferred", "reason": "Later"}}},
				{"empty-dispute", []map[string]any{{"finding": responses[0]["finding"], "response": "disputed", "reason": "  "}}},
				{"control-reason", []map[string]any{{"finding": responses[0]["finding"], "response": "disputed", "reason": "bad\x01"}}},
				{"authority", []map[string]any{{"finding": responses[0]["finding"], "response": "fixed", "authority": "not-authorized"}}},
			}
			for _, c := range invalid {
				refused := hostCall(t, bin, repo, home, v, "review", "invalid-"+c.name, map[string]any{"grant": "round-2", "responses": c.responses})
				if refused["outcome"] != "refused" || refused["token"] != v["token"] {
					t.Fatalf("%s must refuse without mutation: %v", c.name, refused)
				}
			}
			refused := hostCall(t, bin, repo, home, v, "review", "non-host", map[string]any{"host": nil, "grant": "round-2", "responses": responses})
			if refused["outcome"] != "refused" {
				t.Fatalf("non-host responses: %v", refused)
			}
		}
		previous := v
		extra := map[string]any{"grant": fmt.Sprintf("round-%d", round), "responses": responses}
		action := fmt.Sprintf("round-%d", round)
		admitted := hostCall(t, bin, repo, home, previous, "review", action, extra)
		if admitted["outcome"] != "ok" {
			t.Fatalf("host responses must be admitted: %v", admitted)
		}
		if replay := hostCall(t, bin, repo, home, previous, "review", action, extra); replay["token"] != admitted["token"] {
			t.Fatalf("replay: %v", replay)
		}
		v = hostSettle(t, bin, repo, home)
		result := v["host_result"].(map[string]any)
		checks, _ := result["resolutions"].([]any)
		if len(checks) != 2 {
			t.Fatalf("host must receive both resolutions: %v", result)
		}
		for i, c := range checks {
			check := c.(map[string]any)
			want := "confirmed"
			if round == 2 {
				want = "rejected"
			}
			if check["finding"] != responses[i]["finding"] || check["check"] != want {
				t.Fatalf("resolution: %v", check)
			}
		}
		if round == 2 {
			latest := result["findings"].([]any)
			if len(latest) != len(responses) {
				t.Fatalf("rejected responses must retain both findings: %v", result)
			}
			for i, finding := range latest {
				responses[i]["finding"] = finding.(map[string]any)["id"]
			}
		}
		if before != git(t, repo, "status", "--porcelain") || refs != git(t, repo, "show-ref") {
			t.Fatal("review modified author worktree or refs")
		}
	}
	refused := hostCall(t, bin, repo, home, v, "review", "obsolete-responses", map[string]any{"grant": "round-4", "responses": responses})
	if refused["outcome"] != "refused" || refused["token"] != v["token"] {
		t.Fatalf("findings from before the latest settled review must be refused: %v", refused)
	}
}

func TestHostRereviewReceivesPriorFindingWithoutResponse(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("trust = os.Getenv(\"APP_MODE\")\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	installFake(t, home, "")
	scriptPath := filepath.Join(filepath.Dir(home), "fake-pi")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	defaultAnswer := "v={'head':b['Head'],'verdict':'findings','coverage':b['Paths'],'findings':[{'id':'model-id','priority':'P2','origin':'policy','file':'sample.txt','start':1,'end':1,'explanation':'Changed prose is redundant','action':'Remove repetition','rule':'unslop/plain-prose','head':b['Head']}]}"
	answer := `prior=b.get('PriorFindings',[])
v={'head':b['Head'],'verdict':'findings','coverage':b['Paths'],'findings':[]}
if prior:
 assert len(prior)==1
 assert prior[0]['priority']=='P1'
 assert prior[0]['file']=='sample.txt'
 assert prior[0]['state']=='fixed-by-later-commit'
 v['findings']=[{'id':'ignored','priority':'P1','origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':'Restore the trusted environment value','action':'Restore APP_MODE','rule':'','head':b['Head'],'conflicts':[prior[0]['id']]}]
else:
 v['findings']=[{'id':'ignored','priority':'P1','origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':'Environment values are untrusted','action':'Remove APP_MODE','rule':'','head':b['Head']}]`
	if !strings.Contains(string(script), defaultAnswer) {
		t.Fatal("fake reviewer answer template not found")
	}
	if err := os.WriteFile(scriptPath, []byte(strings.Replace(string(script), defaultAnswer, answer, 1)), 0700); err != nil {
		t.Fatal(err)
	}
	v := hostPrepare(t, bin, repo, home, axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"fake-host","id":"task-7","metadata":{"worker":"author-1"}}}`), target, "-environment")
	v = hostCall(t, bin, repo, home, v, "review", "environment-review", map[string]any{"grant": "environment-review"})
	if v["outcome"] != "ok" {
		t.Fatalf("first review: %v", v)
	}
	v = hostSettleWithFindings(t, bin, repo, home)
	first := v["host_result"].(map[string]any)["findings"].([]any)[0].(map[string]any)
	if first["priority"] != "P1" {
		t.Fatalf("first finding: %v", first)
	}
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("trust = \"safe\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "-c", "user.name=Author", "-c", "user.email=author@example.invalid", "commit", "-am", "remove environment trust")
	v = hostCall(t, bin, repo, home, v, "new", "rereview-successor", nil)
	v = hostPrepare(t, bin, repo, home, v, target, "-rereview")
	v = hostCall(t, bin, repo, home, v, "review", "rereview", map[string]any{"grant": "rereview"})
	if v["outcome"] != "ok" {
		t.Fatalf("rereview: %v", v)
	}
	v = hostSettle(t, bin, repo, home)
	result := v["host_result"].(map[string]any)
	conflicts, _ := result["conflicts"].([]any)
	findings, _ := result["findings"].([]any)
	if len(conflicts) != 1 || len(findings) != 0 {
		t.Fatalf("reversing the earlier fix must be reported as a non-blocking conflict: %v", result)
	}
	conflict := conflicts[0].(map[string]any)
	if conflict["earlier"].(map[string]any)["id"] != first["id"] || conflict["finding"].(map[string]any)["action"] != "Restore APP_MODE" {
		t.Fatalf("host result omitted the conflict pair: %v", conflict)
	}
	run := v["run"].(map[string]any)
	review := run["review"].(map[string]any)
	resultFromInspect := review["result"].(map[string]any)
	if len(resultFromInspect["conflicts"].([]any)) != 1 || run["status"] == "outstanding-findings" {
		t.Fatalf("inspect did not expose the non-blocking conflict decision: %v", run)
	}
	captured, err := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reviewInput struct {
		Input struct {
			PriorFindings []PriorFinding
		} `json:"input"`
	}
	if err := json.Unmarshal(captured, &reviewInput); err != nil {
		t.Fatal(err)
	}
	if len(reviewInput.Input.PriorFindings) != 1 || reviewInput.Input.PriorFindings[0].ID != first["id"] || reviewInput.Input.PriorFindings[0].State != "fixed-by-later-commit" {
		t.Fatalf("rereview did not receive the earlier finding: %+v", reviewInput.Input.PriorFindings)
	}
}

func TestHostSuccessorInheritsFindingsFromExistingReview(t *testing.T) {
	prior := &Run{
		ID: "prior", Host: &Host{Name: "host", ID: "1"},
		Review: &Execution{Status: "outstanding-findings", Result: &Verdict{Findings: []Finding{{ID: "existing-finding"}}}},
	}
	next := &Run{ID: "next"}
	hostSuccessor(prior, next, prior.Host)
	if len(next.HostFindings) != 1 || next.HostFindings[0].ID != "existing-finding" {
		t.Fatalf("successor lost findings from a review stored before host responses were supported: %v", next.HostFindings)
	}
}

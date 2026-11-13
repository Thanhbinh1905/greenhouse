package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	v = hostSettle(t, bin, repo, home)
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
		if before != git(t, repo, "status", "--porcelain") || refs != git(t, repo, "show-ref") {
			t.Fatal("review modified author worktree or refs")
		}
	}
	refused := hostCall(t, bin, repo, home, v, "review", "obsolete-responses", map[string]any{"grant": "round-4", "responses": responses})
	if refused["outcome"] != "refused" || refused["token"] != v["token"] {
		t.Fatalf("findings from before the latest settled review must be refused: %v", refused)
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

package engine

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func confirmationFake(initial, confirm string) string {
	return fmt.Sprintf(`
def emit(v):
 print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
 print(json.dumps({'type':'agent_settled','aborted':False}))
 sys.exit(0)
def finding(priority, explanation):
 return {'id':'model-id','priority':priority,'origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':explanation,'action':'Fix the line','rule':'','head':b['Head']}
def verdict(findings):
 return {'head':b['Head'],'verdict':'findings' if findings else 'clean','coverage':b['Paths'],'findings':findings}
if 'Independently verify' in b['Intent']:
 emit(%s)
emit(%s)
`, confirm, initial)
}

func runConfirmation(t *testing.T, initial, confirm string) (map[string]any, int) {
	t.Helper()
	bin, repo, home, v := prepareFixture(t)
	invocations := installFake(t, home, confirmationFake(initial, confirm))
	mutate(t, bin, repo, home, v, "review", "review", nil)
	run := runOf(t, waitReview(t, bin, repo, home))
	data, _ := os.ReadFile(invocations)
	return run, strings.Count(string(data), "invoked")
}

func reviewResult(t *testing.T, run map[string]any) map[string]any {
	t.Helper()
	return run["review"].(map[string]any)["result"].(map[string]any)
}

func TestBlockingFindingIsConfirmedByASecondPass(t *testing.T) {
	run, calls := runConfirmation(t,
		"verdict([finding('P1','Line one is wrong')])",
		"verdict([finding('P1','Line one is wrong because the parser rejects it')])")
	if calls != 2 || run["status"] != "outstanding-findings" {
		t.Fatalf("calls=%d status=%v", calls, run["status"])
	}
	findings := reviewResult(t, run)["findings"].([]any)
	f := findings[0].(map[string]any)
	c := f["confirmation"].(map[string]any)
	if len(findings) != 1 || f["priority"] != "P1" || c["status"] != "confirmed" || !strings.Contains(c["evidence"].(string), "parser rejects") {
		t.Fatalf("unexpected findings: %v", findings)
	}
}

func TestBlockingFindingTheSecondPassCannotReproduceIsDropped(t *testing.T) {
	run, calls := runConfirmation(t, "verdict([finding('P1','Line one is wrong')])", "verdict([])")
	result := reviewResult(t, run)
	if calls != 2 || run["status"] != "clean-review" || result["verdict"] != "clean" || len(result["findings"].([]any)) != 0 {
		t.Fatalf("calls=%d status=%v result=%v", calls, run["status"], result)
	}
	dismissed := result["dismissed"].([]any)
	c := dismissed[0].(map[string]any)["confirmation"].(map[string]any)
	if len(dismissed) != 1 || c["status"] != "dropped" || c["reported_priority"] != "P1" {
		t.Fatalf("unexpected dismissed findings: %v", dismissed)
	}
}

func TestBlockingFindingTheSecondPassRatesLowerIsDemoted(t *testing.T) {
	run, _ := runConfirmation(t, "verdict([finding('P1','Line one is wrong')])", "verdict([finding('P2','Only an edge case')])")
	f := reviewResult(t, run)["findings"].([]any)[0].(map[string]any)
	c := f["confirmation"].(map[string]any)
	if f["priority"] != "P2" || c["status"] != "demoted" || c["reported_priority"] != "P1" || run["status"] != "outstanding-findings" {
		t.Fatalf("unexpected finding: %v status=%v", f, run["status"])
	}
}

func TestBlockingFindingStaysBlockingWhenTheSecondPassDoesNotSettle(t *testing.T) {
	run, calls := runConfirmation(t,
		"verdict([finding('P1','Line one is wrong')])",
		"{'head':b['Head'],'verdict':'inconclusive','coverage':[],'findings':[]}")
	f := reviewResult(t, run)["findings"].([]any)[0].(map[string]any)
	c := f["confirmation"].(map[string]any)
	if calls != 2 || f["priority"] != "P1" || c["status"] != "unconfirmed" {
		t.Fatalf("calls=%d finding=%v", calls, f)
	}
}

func TestNonBlockingFindingsAreNotConfirmed(t *testing.T) {
	run, calls := runConfirmation(t, "verdict([finding('P2','Edge case')])", "verdict([])")
	f := reviewResult(t, run)["findings"].([]any)[0].(map[string]any)
	if calls != 1 || f["confirmation"] != nil {
		t.Fatalf("calls=%d finding=%v", calls, f)
	}
}

func TestConfirmationArgsFollowTheAdmittedExecutionMode(t *testing.T) {
	config := ReviewerConfig{Backend: BackendClaude, Model: "fixture/model"}
	schema := verdictSchema(false, false)
	got, err := confirmationArgs(&Execution{Config: config})
	if err != nil || Digest(got) != Digest(legacyAgentArgs(config, "reviewer", schema, reviewerPrompt)) {
		t.Fatalf("legacy execution args = %v, %v", got, err)
	}
	config.Native = true
	want, err := agentArgs(config, "reviewer", schema, reviewerPrompt)
	got, gotErr := confirmationArgs(&Execution{Config: config})
	if err != nil || gotErr != nil || Digest(got) != Digest(want) {
		t.Fatalf("native execution args = %v, %v", got, gotErr)
	}
}

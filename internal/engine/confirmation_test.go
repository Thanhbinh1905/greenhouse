package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestBlockingFindingStaysBlockingWhenTheSecondPassDoesNotCoverItsFile(t *testing.T) {
	run, _ := runConfirmation(t,
		"verdict([finding('P1','Line one is wrong')])",
		"{'head':b['Head'],'verdict':'clean','coverage':[],'findings':[]}")
	result := reviewResult(t, run)
	f := result["findings"].([]any)[0].(map[string]any)
	c := f["confirmation"].(map[string]any)
	if f["priority"] != "P1" || c["status"] != "unconfirmed" || result["dismissed"] != nil || run["status"] != "outstanding-findings" {
		t.Fatalf("finding=%v status=%v", f, run["status"])
	}
}

func TestBlockingFindingIsNotDemotedByAPassThatDoesNotCoverItsFile(t *testing.T) {
	run, _ := runConfirmation(t,
		"verdict([finding('P1','Line one is wrong')])",
		"{'head':b['Head'],'verdict':'findings','coverage':[],'findings':[finding('P2','Only an edge case')]}")
	f := reviewResult(t, run)["findings"].([]any)[0].(map[string]any)
	c := f["confirmation"].(map[string]any)
	if f["priority"] != "P1" || c["status"] != "unconfirmed" {
		t.Fatalf("finding=%v", f)
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

func TestDismissedFindingLeavesTheRunLedger(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	installFake(t, home, confirmationFake("verdict([finding('P1','Line one is wrong')])", "verdict([] if os.path.exists(root+'/drop') else [finding('P1','Line one is wrong')])"))
	b, _ := json.Marshal(map[string]any{"version": 2, "op": "attach", "host": fakeHost})
	v := hostPrepare(t, bin, repo, home, axi(t, bin, repo, home, string(b)), target, "")
	hostCall(t, bin, repo, home, v, "review", "review-1", map[string]any{"grant": "round-1"})
	v = hostSettle(t, bin, repo, home)
	if findings, _ := runOf(t, v)["findings"].([]any); len(findings) != 1 {
		t.Fatalf("first review should keep the confirmed finding: %v", v)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(home), "drop"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	hostCall(t, bin, repo, home, v, "review", "review-2", map[string]any{"grant": "round-2"})
	v = hostSettle(t, bin, repo, home)
	result := v["host_result"].(map[string]any)
	if findings, _ := runOf(t, v)["findings"].([]any); len(findings) != 0 || len(result["dismissed_findings"].([]any)) != 1 || result["verdict"] != "clean" {
		t.Fatalf("a dismissed finding must leave the run ledger: %v", v)
	}
}

func TestConfirmationPassesKeepTheirOwnExecutorRecords(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, confirmationFake("verdict([finding('P1','Line one is wrong')])", "verdict([finding('P1','Line one is wrong')])"))
	mutate(t, bin, repo, home, v, "review", "review", nil)
	id := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)["id"].(string)
	verdict, _ := filepath.Glob(filepath.Join(home, "executors", id+".*.lease"))
	confirmation, _ := filepath.Glob(filepath.Join(home, "executors", id+"-confirm-*.lease"))
	if len(verdict) != 1 || len(confirmation) != 1 {
		t.Fatalf("verdict records %v, confirmation records %v", verdict, confirmation)
	}
}

func TestMarkUnconfirmedKeepsBlockingFindingsBlocking(t *testing.T) {
	done := reviewOutcome{Status: "outstanding-findings", Result: &Verdict{Findings: []Finding{{Priority: "P1"}, {Priority: "P2"}, {Priority: "P0", Confirmation: &Confirmation{Status: "confirmed"}}}}}
	markUnconfirmed(&done)
	findings := done.Result.Findings
	if findings[0].Confirmation == nil || findings[0].Confirmation.Status != "unconfirmed" || findings[0].Priority != "P1" || findings[1].Confirmation != nil || findings[2].Confirmation.Status != "confirmed" {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestRestartDuringConfirmationAdoptsTheVerdictAsUnconfirmed(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	finding := "verdict([finding('P1','Line one is wrong')])"
	mode := strings.Replace(confirmationFake(finding, finding), "if 'Independently verify' in b['Intent']:\n", "if 'Independently verify' in b['Intent']:\n open(root+'/confirming','w').close()\n while not os.path.exists(root+'/release'): time.sleep(0.01)\n", 1)
	installFake(t, home, mode)
	mutate(t, bin, repo, home, v, "review", "review", nil)
	root := filepath.Dir(home)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "confirming")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("confirmation pass did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	pidBytes, err := os.ReadFile(filepath.Join(home, "pid"))
	var pid int
	if err != nil || json.Unmarshal(pidBytes, &pid) != nil || pid <= 0 {
		t.Fatal("invalid fixture daemon identity")
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	run := runOf(t, waitLeave(t, bin, repo, home, "recovery-required"))
	findings, _ := run["findings"].([]any)
	if run["status"] != "outstanding-findings" || len(findings) != 1 {
		t.Fatalf("restart did not adopt the verdict: %v", run)
	}
	f := findings[0].(map[string]any)
	if f["priority"] != "P1" || f["confirmation"].(map[string]any)["status"] != "unconfirmed" {
		t.Fatalf("finding = %v", f)
	}
}

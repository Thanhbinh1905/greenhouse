package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var fakeHost = map[string]any{"name": "fake-host", "id": "task-7", "metadata": map[string]string{"worker": "author-1"}}

func hostCall(t *testing.T, bin, repo, home string, v map[string]any, op, action string, extra map[string]any) map[string]any {
	t.Helper()
	q := map[string]any{"version": 2, "op": op, "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": action, "reason": "fake host", "host": fakeHost}
	for k, value := range extra {
		q[k] = value
	}
	b, _ := json.Marshal(q)
	return axi(t, bin, repo, home, string(b))
}

func hostSettle(t *testing.T, bin, repo, home string) map[string]any {
	t.Helper()
	waitReview(t, bin, repo, home)
	b, _ := json.Marshal(map[string]any{"version": 2, "op": "attach", "host": fakeHost})
	return axi(t, bin, repo, home, string(b))
}

func hostPrepare(t *testing.T, bin, repo, home string, v map[string]any, target, suffix string) map[string]any {
	t.Helper()
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 1, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	for _, step := range []struct {
		op    string
		extra map[string]any
	}{{"apply", map[string]any{"settings": settings}}, {"prepare", map[string]any{"intent": "Review sample change", "target": target}}, {"resolve-rules", nil}} {
		v = hostCall(t, bin, repo, home, v, step.op, step.op+suffix, step.extra)
		if v["outcome"] != "ok" {
			t.Fatalf("host %s: %v", step.op, v)
		}
	}
	return v
}

func TestEmbeddedHostOwnsDelivery(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	remote := filepath.Join(filepath.Dir(repo), "remote.git")
	git(t, filepath.Dir(repo), "init", "--bare", remote)
	git(t, repo, "remote", "add", "origin", remote)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	b, _ := json.Marshal(map[string]any{"version": 2, "op": "attach", "host": fakeHost})
	v := axi(t, bin, repo, home, string(b))
	invocations := installFake(t, home, "")
	writeFixerConfig(t, home, FixerConfig{Executable: filepath.Join(filepath.Dir(home), "fake-pi"), Model: "fixture/model", Name: "Fixer", Email: "fixer@example.invalid", Checks: [][]string{{"/bin/true"}}})
	host, _ := runOf(t, v)["host"].(map[string]any)
	if v["outcome"] != "ok" || host["name"] != "fake-host" || host["id"] != "task-7" {
		t.Fatalf("attach must record host identity: %v", v)
	}

	observer := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if observer["run_id"] != v["run_id"] || strings.Join(stringsOf(observer["actions"]), ",") != "attach,inspect" || !strings.Contains(strings.Join(stringsOf(observer["blocked"]), "\n"), "owned by host fake-host task-7") {
		t.Fatalf("a standalone client may only observe a host run: %v", observer)
	}
	if refused := mutate(t, bin, repo, home, observer, "apply", "standalone-apply", map[string]any{"mode": "ask", "preset": "full", "limit": 9, "rules": []string{}}); refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "owned by host") {
		t.Fatalf("standalone mutation of a host run: %v", refused)
	}

	v = hostPrepare(t, bin, repo, home, v, target, "-1")
	if missing := hostCall(t, bin, repo, home, v, "review", "review-no-grant", nil); missing["outcome"] != "refused" || !strings.Contains(missing["error"].(string), "grant") {
		t.Fatalf("host review needs a host grant: %v", missing)
	}
	first := hostCall(t, bin, repo, home, v, "review", "review-1", map[string]any{"grant": "round-1"})
	if running, _ := first["host_result"].(map[string]any); first["outcome"] != "ok" || running["status"] != "running" || running["current"] != true {
		t.Fatalf("a running review of the unchanged head is current: %v", first)
	}
	if replay := hostCall(t, bin, repo, home, v, "review", "review-1", map[string]any{"grant": "round-1"}); replay["token"] != first["token"] {
		t.Fatalf("replay must return the receipt: %v", replay)
	}
	v = hostSettle(t, bin, repo, home)
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	result, _ := v["host_result"].(map[string]any)
	findings, _ := result["findings"].([]any)
	if result["grant"] != "round-1" || result["head"] != head || result["target"] != target || result["review_base"] != target || result["verdict"] != "findings" || result["current"] != true || len(findings) != 1 || findings[0].(map[string]any)["head"] != head {
		t.Fatalf("host result must carry exact-head findings and verdict: %v", v)
	}
	term := startTerminal(t, bin, repo, home)
	term.waitFor(t, "Host: fake-host task-7 | grant round-1 | findings | current true")
	io.WriteString(term.stdin, "v\n")
	term.waitFor(t, "refused run is owned by host fake-host task-7")
	if again := hostCall(t, bin, repo, home, v, "review", "review-1-again", map[string]any{"grant": "round-1"}); again["outcome"] != "refused" || !strings.Contains(again["error"].(string), "grant already used") {
		t.Fatalf("one grant must buy one review: %v", again)
	}

	for _, op := range []string{"repair", "resolve-conflict", "pipeline", "handback", "decide", "rerun-from-start", "save"} {
		refused := hostCall(t, bin, repo, home, v, op, "host-"+op, map[string]any{"selected": FindingIDs(nil), "choice": "accept-head"})
		if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "the host owns") {
			t.Fatalf("host mode must delegate %s: %v", op, refused)
		}
		if has(stringsOf(v["actions"]), op) {
			t.Fatalf("host run offers %s: %v", op, v["actions"])
		}
	}

	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("reviewed and fixed by the host author\n"), 0600)
	git(t, repo, "-c", "user.name=Author", "-c", "user.email=author@example.invalid", "commit", "-am", "author repair")
	moved := hostSettle(t, bin, repo, home)
	if moved["host_result"].(map[string]any)["current"] != false {
		t.Fatalf("a verdict for a moved head must not be current: %v", moved)
	}
	next := hostCall(t, bin, repo, home, moved, "new", "host-new", nil)
	successor := runOf(t, next)
	if next["outcome"] != "ok" || successor["predecessor"] != v["run_id"] || successor["review_used"] != float64(1) || successor["host"].(map[string]any)["id"] != "task-7" {
		t.Fatalf("a host successor must keep the lineage and usage: %v", next)
	}
	next = hostPrepare(t, bin, repo, home, next, target, "-2")
	if reused := hostCall(t, bin, repo, home, next, "review", "review-2-reused", map[string]any{"grant": "round-1"}); reused["outcome"] != "refused" || !strings.Contains(reused["error"].(string), "grant already used") {
		t.Fatalf("a successor must not reuse a spent grant: %v", reused)
	}
	if second := hostCall(t, bin, repo, home, next, "review", "review-2", map[string]any{"grant": "round-2"}); second["outcome"] != "ok" {
		t.Fatalf("the host cap, not the Greenhouse limit of 1, decides the second review: %v", second)
	}
	final := hostSettle(t, bin, repo, home)
	newHead := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if r := final["host_result"].(map[string]any); r["grant"] != "round-2" || r["head"] != newHead || r["current"] != true || runOf(t, final)["review_used"] != float64(2) || final["decision"] != nil {
		t.Fatalf("second round: %v", final)
	}
	for _, blocked := range stringsOf(final["blocked"]) {
		if strings.Contains(blocked, "budget exhausted") || strings.Contains(blocked, "selected repair unavailable") {
			t.Fatalf("Greenhouse made a cap decision for the host: %v", final["blocked"])
		}
	}

	term.waitFor(t, "grant round-2")
	io.WriteString(term.stdin, "q\n")
	if err := <-term.done; err != nil {
		t.Fatalf("terminal detach: %v", err)
	}
	calls, _ := os.ReadFile(invocations)
	if n := strings.Count(string(calls), "invoked"); n != 2 {
		t.Fatalf("expected exactly two reviewer invocations and no fixer, got %d", n)
	}
	if refs := git(t, repo, "for-each-ref", "refs/greenhouse"); refs != "" {
		t.Fatalf("host mode created candidate refs: %s", refs)
	}
	if published := git(t, remote, "for-each-ref"); published != "" {
		t.Fatalf("host mode published: %s", published)
	}
	if worktrees := git(t, repo, "worktree", "list", "--porcelain"); strings.Count(worktrees, "worktree ") != 1 {
		t.Fatalf("host mode created a private worktree: %s", worktrees)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "greenhouse-leases")); !os.IsNotExist(err) {
		t.Fatalf("host mode took candidate custody: %v", err)
	}
}

func stringsOf(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, item := range list {
		out = append(out, item.(string))
	}
	return out
}

func TestHostOwnershipBoundaries(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	standalone := axi(t, bin, repo, home, `{"version":2,"op":"attach"}`)
	b, _ := json.Marshal(map[string]any{"version": 2, "op": "attach", "host": fakeHost})
	if taken := axi(t, bin, repo, home, string(b)); taken["outcome"] != "refused" || !strings.Contains(taken["error"].(string), "standalone run owns") || runOf(t, taken)["host"] != nil {
		t.Fatalf("a host must not silently adopt a standalone run: %v", taken)
	}
	replaced := hostCall(t, bin, repo, home, standalone, "new", "host-takes-over", nil)
	if replaced["outcome"] != "ok" || runOf(t, replaced)["host"] == nil || runOf(t, replaced)["review_used"] != float64(0) {
		t.Fatalf("an explicit host new must replace the standalone run: %v", replaced)
	}
	other := map[string]any{"version": 2, "op": "attach", "host": map[string]any{"name": "other-host", "id": "task-9"}}
	b, _ = json.Marshal(other)
	if refused := axi(t, bin, repo, home, string(b)); refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "owned by host fake-host task-7") {
		t.Fatalf("a second host must not share the run: %v", refused)
	}
	if invalid := axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"","id":"x"}}`); invalid["outcome"] != "refused" {
		t.Fatalf("an empty host name must refuse: %v", invalid)
	}
}

func TestHostReviewRecoveryPreservesGrantAcrossChoices(t *testing.T) {
	for _, choice := range []string{"adopt", "discard", "new-run"} {
		t.Run(choice, func(t *testing.T) {
			t.Parallel()
			bin, repo, home, v := prepareFixture(t)
			root := filepath.Dir(home)
			fake := filepath.Join(root, "fake-pi")
			writeExecutable(t, fake, `#!/usr/bin/python3
import json,os,sys,time
if '--list-models' in sys.argv:
 print('fixture model')
 sys.exit(0)
root=os.path.dirname(__file__)
open(root+'/reviewer-pid','w').write(str(os.getpid()))
open(root+'/runner-pid','w').write(str(os.getppid()))
while not os.path.exists(root+'/release-review'):
 time.sleep(0.01)
b=json.load(sys.stdin)
v={'head':b['Head'],'verdict':'findings','coverage':b['Paths'],'findings':[{'id':'recovery','priority':'P2','origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':'Fixture finding','action':'Inspect the fixture','rule':'','head':b['Head']}]}
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
`)
			v = hostCall(t, bin, repo, home, v, "new", "host-new-recovery-"+choice, nil)
			target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD~1"))
			v = hostPrepare(t, bin, repo, home, v, target, "-recovery-"+choice)
			started := hostCall(t, bin, repo, home, v, "review", "host-review-recovery-"+choice, map[string]any{"grant": "round-1"})
			if started["outcome"] != "ok" {
				t.Fatal(started)
			}
			deadline := time.Now().Add(10 * time.Second)
			pidPath := filepath.Join(root, "reviewer-pid")
			for time.Now().Before(deadline) {
				if _, err := os.Stat(pidPath); err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			pidBytes, err := os.ReadFile(pidPath)
			if err != nil {
				t.Fatalf("reviewer did not start: %v", err)
			}
			var reviewerPID int
			if _, err = fmt.Sscan(string(pidBytes), &reviewerPID); err != nil || reviewerPID <= 0 {
				t.Fatalf("invalid reviewer pid %q: %v", pidBytes, err)
			}
			runnerBytes, err := os.ReadFile(filepath.Join(root, "runner-pid"))
			if err != nil {
				t.Fatal(err)
			}
			var runnerPID int
			if _, err = fmt.Sscan(string(runnerBytes), &runnerPID); err != nil || runnerPID <= 0 {
				t.Fatalf("invalid executor runner pid %q: %v", runnerBytes, err)
			}
			pidBytes, err = os.ReadFile(filepath.Join(home, "pid"))
			if err != nil {
				t.Fatal(err)
			}
			var daemonPID int
			if err = json.Unmarshal(pidBytes, &daemonPID); err != nil || daemonPID <= 0 {
				t.Fatalf("invalid daemon pid %q: %v", pidBytes, err)
			}
			if err = syscall.Kill(daemonPID, syscall.SIGKILL); err != nil {
				t.Fatal(err)
			}
			if choice == "adopt" {
				if err = os.WriteFile(filepath.Join(root, "release-review"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				var adopted map[string]any
				for time.Now().Before(deadline.Add(15 * time.Second)) {
					adopted = axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"fake-host","id":"task-7"}}`)
					run := runOf(t, adopted)
					if run != nil {
						if recoveries, _ := run["recoveries"].([]any); len(recoveries) > 0 {
							break
						}
					}
					time.Sleep(20 * time.Millisecond)
				}
				run := runOf(t, adopted)
				if run == nil {
					t.Fatalf("review recovery did not settle: %v", adopted)
				}
				recoveries, ok := run["recoveries"].([]any)
				if !ok || len(recoveries) != 1 || recoveries[0].(map[string]any)["answer"].(map[string]any)["choice"] != "adopt" {
					t.Fatalf("complete review was not adopted during recovery reconciliation: %v", adopted)
				}
				assertHostGrant(t, adopted, "round-1", 1)
				return
			}
			if err = syscall.Kill(runnerPID, syscall.SIGKILL); err != nil {
				t.Fatal(err)
			}
			if err = syscall.Kill(reviewerPID, syscall.SIGKILL); err != nil {
				t.Fatal(err)
			}
			var recovered map[string]any
			for time.Now().Before(deadline.Add(15 * time.Second)) {
				recovered = axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"fake-host","id":"task-7"}}`)
				if r := runOf(t, recovered); r != nil && r["status"] == "recovery-required" {
					if rec, ok := r["recovery"].(map[string]any); ok && len(rec["options"].([]any)) > 1 {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
			}
			if r := runOf(t, recovered); r["status"] != "recovery-required" || len(r["recovery"].(map[string]any)["options"].([]any)) < 2 {
				t.Fatalf("interrupted host review did not require recovery: %v", recovered)
			}
			choiceResult := hostCall(t, bin, repo, home, recovered, "recover", "host-review-recover-"+choice, map[string]any{"choice": choice})
			if choiceResult["outcome"] != "ok" {
				t.Fatal(choiceResult)
			}
			if choice == "discard" {
				assertHostGrant(t, choiceResult, "round-1", 2)
				return
			}
			next := runOf(t, choiceResult)
			if next["host_grants"].([]any)[0] != "round-1" || next["review_used"] != float64(1) || next["host"].(map[string]any)["id"] != "task-7" {
				t.Fatalf("new-run did not retain the spent host grant and usage: %v", choiceResult)
			}
			v = hostPrepare(t, bin, repo, home, choiceResult, next["target"].(string), "-next")
			if reused := hostCall(t, bin, repo, home, v, "review", "reuse-recovered-grant", map[string]any{"grant": "round-1"}); reused["outcome"] != "refused" || !strings.Contains(reused["error"].(string), "grant already used") {
				t.Fatalf("new-run reused the recovered grant: %v", reused)
			}
			if err = os.WriteFile(filepath.Join(root, "release-review"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			nextReview := hostCall(t, bin, repo, home, v, "review", "new-grant-after-recovery", map[string]any{"grant": "round-2"})
			if nextReview["outcome"] != "ok" {
				t.Fatal(nextReview)
			}
			assertHostGrant(t, hostSettle(t, bin, repo, home), "round-2", 2, "round-1", "round-2")
		})
	}
}

func assertHostGrant(t *testing.T, v map[string]any, grant string, reviews int, expectedGrants ...string) {
	t.Helper()
	if len(expectedGrants) == 0 {
		expectedGrants = []string{grant}
	}
	result := v["host_result"].(map[string]any)
	run := runOf(t, v)
	grants := run["host_grants"].([]any)
	if result["grant"] != grant || run["review_used"] != float64(reviews) || len(grants) != len(expectedGrants) {
		t.Fatalf("host grant or usage changed: result=%v run=%v", result, run)
	}
	for i, want := range expectedGrants {
		if grants[i] != want {
			t.Fatalf("host grant history = %v, want %v", grants, expectedGrants)
		}
	}
}

func TestHostReviewResumesUnderItsGrant(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	root := installAgentCLIs(t, home)
	setModes(t, root, "codex", "quota", "success")
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD~1"))
	v = hostCall(t, bin, repo, home, v, "new", "host-new", nil)
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop"}, "reviewer_backend": "codex", "reviewer_model": "gpt-5.6-luna", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	for _, step := range []struct {
		op    string
		extra map[string]any
	}{{"apply", map[string]any{"settings": settings}}, {"prepare", map[string]any{"intent": "Review sample change", "target": target}}, {"resolve-rules", nil}, {"review", map[string]any{"grant": "round-1"}}} {
		if v = hostCall(t, bin, repo, home, v, step.op, "host-"+step.op, step.extra); v["outcome"] != "ok" {
			t.Fatalf("host %s: %v", step.op, v)
		}
	}
	paused := hostSettle(t, bin, repo, home)
	if r := paused["host_result"].(map[string]any); r["status"] != "paused" || r["grant"] != "round-1" || r["verdict"] != nil {
		t.Fatalf("a quota pause keeps the grant without a verdict: %v", paused)
	}
	if resumed := hostCall(t, bin, repo, home, paused, "resume", "host-resume", nil); resumed["outcome"] != "ok" {
		t.Fatalf("host resume: %v", resumed)
	}
	done := hostSettle(t, bin, repo, home)
	if r := done["host_result"].(map[string]any); r["grant"] != "round-1" || r["verdict"] != "clean" || runOf(t, done)["review_used"] != float64(1) {
		t.Fatalf("the resumed review must answer the same grant once: %v", done)
	}
	if calls := agentCalls(t, root, "codex"); len(calls) != 2 {
		t.Fatalf("expected the paused and resumed calls, got %d", len(calls))
	}
}

func TestHostReviewMovedTarget(t *testing.T) {
	bin, repo, home := fixture(t)
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "checkout", "-b", "candidate")
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	git(t, repo, "checkout", "main")
	if err := os.WriteFile(filepath.Join(repo, "target-only.txt"), []byte("target advance\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "target-only.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "advance target")
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "checkout", "candidate")
	installFake(t, home, "")
	v := axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"fake-host","id":"task-7"}}`)
	v = hostPrepare(t, bin, repo, home, v, "main", "-moved")
	run := runOf(t, v)
	if diff, _ := run["selected_diff"].(string); !strings.Contains(diff, "+review me") || strings.Contains(diff, "target-only.txt") {
		t.Fatalf("prepare included reverse target changes: %s", diff)
	}
	if run["target"] != target || run["review_base"] != base {
		t.Fatalf("prepare must retain target and report merge-base: target=%v review_base=%v", run["target"], run["review_base"])
	}
	v = hostCall(t, bin, repo, home, v, "review", "review-moved", map[string]any{"grant": "round-moved"})
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	v = hostSettle(t, bin, repo, home)
	run = runOf(t, v)
	review := run["review"].(map[string]any)
	if diff := review["diff"].(string); strings.Contains(diff, "target-only.txt") || diff != run["selected_diff"] {
		t.Fatalf("review included reverse target changes: %s", diff)
	}
	if paths := stringsOf(review["paths"]); strings.Join(paths, ",") != "sample.txt" {
		t.Fatalf("review paths include target changes: %v", paths)
	}
	captured, err := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Input struct {
			Target string
			Files  []struct{ Path string }
		}
	}
	if err := json.Unmarshal(captured, &input); err != nil {
		t.Fatal(err)
	}
	if input.Input.Target != target || len(input.Input.Files) != 1 || input.Input.Files[0].Path != "sample.txt" {
		t.Fatalf("review bundle changed Target or included target files: %s", captured)
	}
	result := v["host_result"].(map[string]any)
	if result["target"] != target || result["review_base"] != base || result["verdict"] != "findings" {
		t.Fatalf("host result must report target and effective review base: %v", result)
	}
}

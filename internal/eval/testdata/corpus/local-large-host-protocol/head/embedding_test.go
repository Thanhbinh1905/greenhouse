package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if result["grant"] != "round-1" || result["head"] != head || result["target"] != target || result["verdict"] != "findings" || result["current"] != true || len(findings) != 1 || findings[0].(map[string]any)["head"] != head {
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
		refused := hostCall(t, bin, repo, home, v, op, "host-"+op, map[string]any{"selected": findingIDs(nil), "choice": "accept-head"})
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

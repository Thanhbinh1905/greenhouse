package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func workerCommand(t *testing.T, bin, repo, home string, args ...string) map[string]any {
	t.Helper()
	c := exec.Command(bin, append([]string{"axi"}, args...)...)
	c.Dir = repo
	c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "HOME="+filepath.Dir(home), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GREENHOUSE_TEST_PROVIDER_KEY=fixture-secret", "PATH="+filepath.Join(filepath.Dir(home), "native-bin")+":"+os.Getenv("PATH"), "PI_CODING_AGENT_DIR="+filepath.Join(filepath.Dir(home), "native-agent"), "GORACE=atexit_sleep_ms=0")
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("axi %v: %v: %s", args, err, b)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("axi %v: %v: %s", args, err, b)
	}
	if v["outcome"] != "ok" {
		t.Fatalf("axi %v refused: %s", args, b)
	}
	return v
}

func TestWorkerLogsProtocol(t *testing.T) {
	t.Parallel()
	bin, repo, home, _ := prepareFixture(t)
	v := axi(t, bin, repo, home, `{"version":2,"op":"logs","step":"review","full":true}`)
	if v["outcome"] != "ok" || v["logs"] == nil {
		t.Fatalf("logs must return a readable empty stream before dispatch: %v", v)
	}
}

func TestWorkerSubcommands(t *testing.T) {
	t.Parallel()
	bin, repo, home, _ := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
	})
	state := workerCommand(t, bin, repo, home, "inspect")
	if state["run_id"] == nil {
		t.Fatal(state)
	}
	state = workerCommand(t, bin, repo, home, "pipeline", "--intent", "Change the sample", "--wait", "30s")
	if runOf(t, state)["status"] != "needs-decision" {
		t.Fatalf("bounded pipeline must stop at missing delivery configuration: %v", state)
	}
	logs := workerCommand(t, bin, repo, home, "logs", "--step", "test", "--full")["logs"].(map[string]any)
	if logs["content"] == "" {
		t.Fatal("test output is missing")
	}
}

func TestWorkerLogFollowAndRedaction(t *testing.T) {
	t.Parallel()
	bin, repo, home, _ := pipelineFixture(t, func(root string) map[string]any {
		writeExecutable(t, filepath.Join(root, "stream-gate"), "#!/usr/bin/python3\nimport sys,time\nprint('first line',flush=True)\ntime.sleep(1)\nprint('token=abcdefgh1234567890',flush=True)\n")
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{{filepath.Join(root, "stream-gate")}}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
	})
	state := workerCommand(t, bin, repo, home, "pipeline", "--wait", "1ms")
	if state["wait"] != "elapsed" || runOf(t, state)["status"] != "running" {
		t.Fatalf("bounded wait lost running state: %v", state)
	}
	var content string
	offset := 0
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		v := workerCommand(t, bin, repo, home, "logs", "--step", "test", "--offset", fmt.Sprint(offset), "--follow", "--wait", "2s")
		log := v["logs"].(map[string]any)
		content += log["content"].(string)
		offset = int(log["next"].(float64))
		if strings.Contains(content, "elapsed_ms=") {
			break
		}
	}
	if !strings.Contains(content, "first line") || !strings.Contains(content, "[REDACTED]") || strings.Contains(content, "abcdefgh1234567890") {
		t.Fatalf("incremental redacted output: %s", content)
	}
	workerCommand(t, bin, repo, home, "transcript")
	waitPipeline(t, bin, repo, home)
}

func TestWorkerDeliveryContract(t *testing.T) {
	var bare, head string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "check_runs": []map[string]string{{"name": "fixture-ci", "status": "completed", "conclusion": "success", "head_sha": head}}})
		case strings.HasSuffix(r.URL.Path, "/pulls/12"):
			json.NewEncoder(w).Encode(map[string]any{"number": 12, "head": map[string]string{"sha": head}})
		case r.Method == http.MethodGet:
			w.Write([]byte(`[]`))
		case r.Method == http.MethodPost:
			var request struct{ Head string }
			json.NewDecoder(r.Body).Decode(&request)
			b, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+strings.TrimPrefix(request.Head, "acme:")).Output()
			if err != nil {
				t.Error(err)
				return
			}
			head = strings.TrimSpace(string(b))
			json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/pull/12", "head": map[string]string{"sha": head}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	setForgeAPI(t, server.URL)
	bin, repo, home, _ := pipelineFixture(t, func(root string) map[string]any {
		bare = filepath.Join(root, "remote.git")
		if err := exec.Command("git", "init", "--bare", bare).Run(); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"name": "Test", "email": "test@example.invalid", "remote": bare, "github_owner": "acme", "github_repo": "garden", "pull_base": "main", "required_checks": []string{"fixture-ci"}, "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
	})
	os.WriteFile(filepath.Join(filepath.Dir(home), "review-mode"), []byte("findings"), 0600)
	state := workerCommand(t, bin, repo, home, "inspect")
	state = mutate(t, bin, repo, home, state, "apply", "ask", map[string]any{"mode": "ask", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
	state = workerCommand(t, bin, repo, home, "pipeline", "--wait", "30s")
	decision, ok := state["decision"].(map[string]any)
	if !ok || decision["kind"] == nil || decision["options"] == nil || state["token"] == nil {
		t.Fatalf("open gate missing: %v", state)
	}
	finding := runOf(t, state)["findings"].([]any)[0].(map[string]any)
	for _, field := range []string{"id", "priority", "file", "start", "end", "explanation", "action"} {
		if finding[field] == nil {
			t.Fatalf("missing finding %s: %v", field, finding)
		}
	}
	eventState := workerCommand(t, bin, repo, home, "watch", "--stream", state["stream"].(string), "--after", "0")
	if eventState["events"] == nil {
		t.Fatal("decision events missing")
	}
	os.WriteFile(filepath.Join(filepath.Dir(home), "fix-then-review"), []byte("clean"), 0600)
	state = workerCommand(t, bin, repo, home, "decide", "repair", "--reason", "Fix the sample")
	for deadline := time.Now().Add(30 * time.Second); runOf(t, state)["status"] == "running" && time.Now().Before(deadline); {
		state = workerCommand(t, bin, repo, home, "watch", "--stream", state["stream"].(string), "--after", fmt.Sprint(state["sequence"]), "--wait", "5s")
	}
	assertDeliveryReady(t, state)
	delivery, ok := state["delivery"].(map[string]any)
	if !ok || delivery["current"] != true || delivery["pushed_head"] != head || delivery["pr_number"] != float64(12) {
		t.Fatalf("delivery evidence missing: %v", state)
	}
	checks := delivery["checks"].([]any)
	if len(checks) != 1 || checks[0].(map[string]any)["conclusion"] != "success" {
		t.Fatalf("required checks missing: %v", delivery)
	}
	for _, step := range []string{"review", "test", "lint", "push", "pr", "ci"} {
		if workerCommand(t, bin, repo, home, "logs", "--step", step, "--full")["logs"].(map[string]any)["content"] == "" {
			t.Fatalf("missing %s output", step)
		}
	}
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "advance source")
	if workerCommand(t, bin, repo, home, "inspect")["delivery"].(map[string]any)["current"] != false {
		t.Fatal("source movement must invalidate delivery evidence")
	}
}

func TestWorkerStartsUnpreparedFullRunAndCancels(t *testing.T) {
	t.Parallel()
	bin, repo, home, _ := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
	})
	initial := workerCommand(t, bin, repo, home, "inspect")
	target := runOf(t, initial)["target"].(string)
	stopDaemonAndWait(t, home)
	b, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var store Store
	if err := json.Unmarshal(b, &store); err != nil {
		t.Fatal(err)
	}
	for _, run := range store.Runs {
		store.Defaults = run.Configs[len(run.Configs)-1].Settings
		store.Defaults.Preset = "one-pass"
	}
	store.Runs = map[string]*Run{}
	store.Slots = map[string]string{}
	store.Receipts = map[string]Receipt{}
	if err := persist(home, &store); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(filepath.Dir(home), "sleep-test"), []byte("5"), 0600)
	state := workerCommand(t, bin, repo, home, "pipeline", "--intent", "Change the sample", "--target", target, "--wait", "1ms")
	if state["wait"] != "elapsed" || runOf(t, state)["intent"] != "Change the sample" {
		t.Fatalf("fresh full run: %v", state)
	}
	for deadline := time.Now().Add(30 * time.Second); runOf(t, state)["stage"] != "test" && time.Now().Before(deadline); {
		state = workerCommand(t, bin, repo, home, "watch", "--stream", state["stream"].(string), "--after", fmt.Sprint(state["sequence"]), "--wait", "5s")
	}
	workerCommand(t, bin, repo, home, "cancel")
	waitPipeline(t, bin, repo, home)
}

func TestWorkerStepEventsAndLogBounds(t *testing.T) {
	t.Parallel()
	bin, repo, home, _ := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
	})
	os.WriteFile(filepath.Join(filepath.Dir(home), "require-test"), []byte("different\n"), 0600)
	cursor := workerCommand(t, bin, repo, home, "watch")
	workerCommand(t, bin, repo, home, "pipeline", "--wait", "30s")
	events := workerCommand(t, bin, repo, home, "watch", "--stream", cursor["stream"].(string), "--after", fmt.Sprint(cursor["sequence"]))["events"].([]any)
	started, finished, failed, park := false, false, false, false
	for _, value := range events {
		event := value.(map[string]any)
		switch event["kind"] {
		case "step-started":
			started = true
			if event["command"] == nil {
				t.Fatal(event)
			}
		case "step-finished":
			finished = true
			if event["exit_code"] != float64(0) || event["elapsed_ms"] == nil {
				t.Fatal(event)
			}
		case "step-failed":
			failed = true
			if event["exit_code"] != float64(1) || event["elapsed_ms"] == nil {
				t.Fatal(event)
			}
		case "park", "decision":
			park = true
		}
	}
	if !started || !finished || !failed || !park {
		t.Fatalf("missing lifecycle events: %v", events)
	}
	for _, request := range []string{
		`{"version":2,"op":"logs","step":"bogus"}`,
		`{"version":2,"op":"logs","step":"test","offset":-1}`,
		`{"version":2,"op":"logs","step":"test","offset":99999999}`,
		`{"version":2,"op":"logs","step":"test","follow":true,"wait_ms":60001}`,
	} {
		if v := axi(t, bin, repo, home, request); v["outcome"] != "refused" {
			t.Fatalf("invalid logs accepted: %v", v)
		}
	}
	transcript := workerCommand(t, bin, repo, home, "transcript")["transcript"].(map[string]any)
	if len(transcript["steps"].([]any)) == 0 {
		t.Fatal("terminal stage logs missing")
	}
}

func TestWorkerPipelineWaitsForCancellationCleanup(t *testing.T) {
	t.Parallel()
	bin, repo, home, _ := pipelineFixture(t, localPlan)
	os.WriteFile(filepath.Join(filepath.Dir(home), "sleep-test"), []byte("30"), 0600)
	waited := make(chan map[string]any, 1)
	go func() { waited <- workerCommand(t, bin, repo, home, "pipeline", "--wait", "30s") }()
	state := workerCommand(t, bin, repo, home, "watch")
	for deadline := time.Now().Add(30 * time.Second); runOf(t, state)["stage"] != "test" && time.Now().Before(deadline); {
		state = workerCommand(t, bin, repo, home, "watch", "--stream", state["stream"].(string), "--after", fmt.Sprint(state["sequence"]), "--wait", "5s")
	}
	if runOf(t, state)["stage"] != "test" {
		t.Fatal("pipeline did not start the test executor")
	}
	workerCommand(t, bin, repo, home, "cancel")
	settled := <-waited
	if settled["wait"] != "settled" || runOf(t, settled)["status"] != "cancelled" {
		t.Fatalf("pipeline wait returned before cancellation cleanup: wait=%v status=%v", settled["wait"], runOf(t, settled)["status"])
	}
}

func TestWorkerPipelineRejectsInvalidIntent(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	before := axi(t, bin, repo, home, `{"version":2,"op":"attach"}`)
	for i, intent := range []string{" \t\n", strings.Repeat("x", 8193), "invalid\x00intent"} {
		request, _ := json.Marshal(map[string]any{"version": 2, "op": "pipeline", "run_id": before["run_id"], "input_id": before["input_id"], "gate_id": before["gate_id"], "expected": before["token"], "action_id": fmt.Sprint("invalid-intent-", i), "intent": intent})
		got := axi(t, bin, repo, home, string(request))
		if got["outcome"] != "refused" || !strings.Contains(fmt.Sprint(got["error"]), "intent") {
			t.Fatalf("invalid intent was not rejected by validation: outcome=%v error=%v", got["outcome"], got["error"])
		}
		after := workerCommand(t, bin, repo, home, "inspect")
		if after["token"] != before["token"] || runOf(t, after)["intent"] != runOf(t, before)["intent"] {
			t.Fatal("invalid intent mutated the run")
		}
	}
}

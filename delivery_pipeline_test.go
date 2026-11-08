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
)

func TestFullPipelineReachesCIReadyWithLocalForge(t *testing.T) {
	var bare, prBranch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet:
			w.Write([]byte(`[]`))
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
			var request struct {
				Head string `json:"head"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			prBranch = strings.TrimPrefix(request.Head, "acme:")
			head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+prBranch).Output()
			if err != nil {
				t.Errorf("read pushed head: %v", err)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]string{"sha": strings.TrimSpace(string(head))}})
		case strings.HasSuffix(r.URL.Path, "/pulls/12") && r.Method == http.MethodGet:
			head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+prBranch).Output()
			if err != nil {
				t.Errorf("read PR head: %v", err)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"number": 12, "head": map[string]string{"sha": strings.TrimSpace(string(head))}})
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			head := strings.Split(r.URL.Path, "/")
			sha := head[len(head)-2]
			json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "check_runs": []map[string]string{{"name": "fixture-ci", "status": "completed", "conclusion": "success", "head_sha": sha}}})
		default:
			t.Errorf("unexpected forge request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	setForgeAPI(t, server.URL)
	bin, repo, home, attached := pipelineFixture(t, func(root string) map[string]any {
		bare = filepath.Join(root, "remote.git")
		if err := exec.Command("git", "init", "--bare", bare).Run(); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"name": "Test", "email": "test@example.invalid", "remote": bare, "github_owner": "acme", "github_repo": "garden", "pull_base": "main", "required_checks": []string{"fixture-ci"}, "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
	})
	started := mutate(t, bin, repo, home, attached, "pipeline", "start", nil)
	if started["outcome"] != "ok" {
		t.Fatal(started)
	}
	done := waitPipeline(t, bin, repo, home)
	assertDeliveryReady(t, done)
	run := runOf(t, done)
	if stageOf(t, done, "pr")["url"] != "https://example.invalid/acme/garden/pull/12" || stageOf(t, done, "pr")["number"] != float64(12) || stageOf(t, done, "ci")["head"] != run["candidate"] {
		t.Fatalf("PR identity or head evidence missing: %v", run["pipeline"])
	}
	if len(stageOf(t, done, "push")["commands"].([]any)) == 0 {
		t.Fatalf("delivery command-runner evidence was lost: %v", stageOf(t, done, "push"))
	}
	verifyDeliveryEvidence(t, home, bare, run)
}

func TestFullPipelineReportsMissingAndStaleChecksThroughAXI(t *testing.T) {
	for _, tc := range []struct{ name, payload, reason string }{
		{"missing", `{"total_count":0,"check_runs":[]}`, "missing required check: fixture-ci"},
		{"stale", `{"total_count":1,"check_runs":[{"name":"fixture-ci","status":"completed","conclusion":"success","head_sha":"stale-head"}]}`, "stale check: fixture-ci"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bare, branch string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet:
					w.Write([]byte(`[]`))
				case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
					var request struct {
						Head string `json:"head"`
					}
					json.NewDecoder(r.Body).Decode(&request)
					branch = strings.TrimPrefix(request.Head, "acme:")
					head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+branch).Output()
					if err != nil {
						t.Errorf("read pushed head: %v", err)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]string{"sha": strings.TrimSpace(string(head))}})
				case r.URL.Path == "/repos/acme/garden/pulls/12":
					head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+branch).Output()
					if err != nil {
						t.Errorf("read PR head: %v", err)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"number": 12, "head": map[string]string{"sha": strings.TrimSpace(string(head))}})
				case strings.HasSuffix(r.URL.Path, "/check-runs"):
					w.Write([]byte(tc.payload))
				default:
					t.Errorf("unexpected forge request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			setForgeAPI(t, server.URL)
			bin, repo, home, attached := pipelineFixture(t, func(root string) map[string]any {
				bare = filepath.Join(root, "remote.git")
				if err := exec.Command("git", "init", "--bare", bare).Run(); err != nil {
					t.Fatal(err)
				}
				return map[string]any{"name": "Test", "email": "test@example.invalid", "remote": bare, "github_owner": "acme", "github_repo": "garden", "pull_base": "main", "required_checks": []string{"fixture-ci"}, "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
			})
			started := mutate(t, bin, repo, home, attached, "pipeline", "start", nil)
			if started["outcome"] != "ok" {
				t.Fatal(started)
			}
			state := waitPipeline(t, bin, repo, home)
			if runOf(t, state)["status"] != "needs-decision" || stageOf(t, state, "ci")["reason"] != tc.reason {
				t.Fatalf("AXI did not report blocked check: %v", runOf(t, state)["pipeline"])
			}
		})
	}
}

func TestFullPipelineTransportExhaustionDoesNotDispatchFixer(t *testing.T) {
	var bare string
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet {
			requests++
			w.Write([]byte(`[]`))
			return
		}
		if r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		t.Errorf("unexpected forge request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	setForgeAPI(t, server.URL)
	bin, repo, home, attached := pipelineFixture(t, func(root string) map[string]any {
		bare = filepath.Join(root, "remote.git")
		if err := exec.Command("git", "init", "--bare", bare).Run(); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"name": "Test", "email": "test@example.invalid", "remote": bare, "github_owner": "acme", "github_repo": "garden", "pull_base": "main", "required_checks": []string{"fixture-ci"}, "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}, "repairs": []string{"ci"}, "repair_limit": 1}
	})
	started := mutate(t, bin, repo, home, attached, "pipeline", "start", nil)
	if started["outcome"] != "ok" {
		t.Fatal(started)
	}
	state := waitPipeline(t, bin, repo, home)
	p := runOf(t, state)["pipeline"].(map[string]any)
	pr := stageOf(t, state, "pr")
	if runOf(t, state)["status"] != "needs-decision" || pr["status"] != "needs-decision" || pr["runs"] != float64(3) || requests != 3 {
		t.Fatalf("transport retries did not exhaust safely: requests=%d stage=%v", requests, pr)
	}
	if countLines(t, filepath.Join(filepath.Dir(home), "fix-invocations"))["ci"] != 0 || p["repair_used"] != float64(0) {
		t.Fatalf("transport error dispatched a fixer: %v", p)
	}
}

func TestFullPipelineRepairsFailedCIWithinBound(t *testing.T) {
	var bare string
	var ciCalls, prCreates, prUpdates int
	branchName := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		refFor := func(branch string) string { return "refs/heads/" + strings.TrimPrefix(branch, "acme:") }
		currentHead := func(branch string) string {
			b, _ := exec.Command("git", "--git-dir", bare, "rev-parse", refFor(branch)).Output()
			return strings.TrimSpace(string(b))
		}
		switch {
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet:
			if prCreates == 0 {
				w.Write([]byte(`[]`))
				return
			}
			branchName = r.URL.Query().Get("head")
			json.NewEncoder(w).Encode([]map[string]any{{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]any{"sha": currentHead(branchName)}}})
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
			prCreates++
			var req struct {
				Head string `json:"head"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			branchName = req.Head
			json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]any{"sha": currentHead(branchName)}})
		case r.Method == http.MethodPatch:
			prUpdates++
			json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]any{"sha": currentHead(branchName)}})
		case strings.HasSuffix(r.URL.Path, "/pulls/12") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{"number": 12, "head": map[string]string{"sha": currentHead(branchName)}})
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			ciCalls++
			split := strings.Split(r.URL.Path, "/")
			sha := split[len(split)-2]
			conclusion := "success"
			if ciCalls == 1 {
				conclusion = "failure"
			}
			json.NewEncoder(w).Encode(map[string]any{"total_count": 2, "check_runs": []map[string]string{{"name": "fixture-ci", "status": "completed", "conclusion": conclusion, "head_sha": sha}, {"name": "optional-lint", "status": "completed", "conclusion": "success", "head_sha": sha}}})
		default:
			t.Errorf("unexpected forge request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	setForgeAPI(t, server.URL)
	bin, repo, home, attached := pipelineFixture(t, func(root string) map[string]any {
		bare = filepath.Join(root, "remote.git")
		if err := exec.Command("git", "init", "--bare", bare).Run(); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"name": "Test", "email": "test@example.invalid", "remote": bare, "github_owner": "acme", "github_repo": "garden", "pull_base": "main", "required_checks": []string{"fixture-ci"}, "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}, "repairs": []string{"ci"}, "repair_limit": 1}
	})
	root := filepath.Dir(home)
	os.WriteFile(filepath.Join(root, "fix-content"), []byte("repaired\n"), 0600)
	started := mutate(t, bin, repo, home, attached, "pipeline", "start", nil)
	if started["outcome"] != "ok" {
		t.Fatal(started)
	}
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	if run["status"] != "ci-ready" {
		t.Fatalf("CI repair did not converge: %v", run["pipeline"])
	}
	p := run["pipeline"].(map[string]any)
	if p["repair_used"] != float64(1) || ciCalls != 2 || prCreates != 1 || prUpdates != 1 {
		t.Fatalf("repair or PR was not idempotent: repairs=%v CI=%d PR create=%d update=%d", p["repair_used"], ciCalls, prCreates, prUpdates)
	}
	if got := countLines(t, filepath.Join(root, "review-invocations")); got["review"] != 2 {
		t.Fatalf("new head not reviewed: %v", got)
	}
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["ci"] != 1 {
		t.Fatalf("CI fixer count %v", got)
	}
	verifyDeliveryEvidence(t, home, bare, run)
}

func TestInterruptedDeliveryStagesReconcileFromDurableState(t *testing.T) {
	for _, name := range []string{"push", "pr", "ci"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			settings := Settings{Mode: "auto", Preset: "full", Limit: 4, Rules: []string{"unslop"}}
			run := &Run{ID: "run", Input: "input", Token: "token", Candidate: "candidate", Status: "running", Configs: []Snapshot{{ID: "config", Settings: settings}}, Pipeline: newPipeline(CommandPlan{}, map[string]string{})}
			run.Pipeline.Status = "running"
			run.Pipeline.Active = &Step{ID: "step", Stage: name, Kind: "delivery"}
			st := run.Pipeline.stage(name)
			st.Status, st.Inputs, st.Candidate, st.Number, st.Runs = "running", "input", "candidate", 42, 2
			store := &Store{Version: 1, Defaults: settings, Runs: map[string]*Run{"run": run}, Slots: map[string]string{}, Receipts: map[string]Receipt{}}
			if err := persist(home, store); err != nil {
				t.Fatal(err)
			}
			restarted, err := load(home)
			if err != nil {
				t.Fatal(err)
			}
			got := restarted.Runs["run"]
			if got.Status != "running" || got.Pipeline.Status != "running" || got.Pipeline.Active != nil || got.Pipeline.stage(name).Status != "pending" || got.Pipeline.stage(name).Inputs != "" {
				t.Fatalf("delivery not reconciled: %+v", got.Pipeline)
			}
		})
	}
}

func TestTransportRetryBudgetIsBounded(t *testing.T) {
	for _, tc := range []struct {
		runs             int
		retry, exhausted bool
	}{
		{1, true, false}, {2, true, false}, {3, false, true},
	} {
		st := &StageResult{Status: "failed", Reason: "check inspection failed: unavailable", Runs: tc.runs}
		retry, exhausted := deliveryRetry(st)
		if retry != tc.retry || exhausted != tc.exhausted {
			t.Errorf("runs=%d: retry=%t exhausted=%t", tc.runs, retry, exhausted)
		}
	}
	if retry, exhausted := deliveryRetry(&StageResult{Status: "failed", Reason: "check failed: ci", Runs: 3}); retry || exhausted {
		t.Fatal("CI check failure was mistaken for transport exhaustion")
	}
}

func setForgeAPI(t *testing.T, value string) {
	t.Helper()
	previous, had := os.LookupEnv("GREENHOUSE_GITHUB_API")
	if err := os.Setenv("GREENHOUSE_GITHUB_API", value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv("GREENHOUSE_GITHUB_API", previous)
		} else {
			os.Unsetenv("GREENHOUSE_GITHUB_API")
		}
	})
}
func assertDeliveryReady(t *testing.T, state map[string]any) {
	t.Helper()
	run := runOf(t, state)
	if run["status"] != "ci-ready" || run["pipeline"].(map[string]any)["status"] != "ci-ready" {
		t.Fatalf("not CI-ready: %v", run["pipeline"])
	}
	for _, stage := range []string{"push", "pr", "ci"} {
		if stageOf(t, state, stage)["status"] != "passed" {
			t.Fatalf("stage %s: %v", stage, stageOf(t, state, stage))
		}
	}
}
func verifyDeliveryEvidence(t *testing.T, home, bare string, run map[string]any) {
	t.Helper()
	p := run["pipeline"].(map[string]any)
	ref := strings.TrimPrefix(p["ref"].(string), "refs/heads/")
	if got := strings.TrimSpace(git(t, bare, "rev-parse", "refs/heads/"+ref)); got != run["candidate"] {
		t.Fatalf("remote %s != candidate %s", got, run["candidate"])
	}
	root := filepath.Dir(home)
	if got := countLines(t, filepath.Join(root, "fix-invocations")); got["test"]+got["lint"]+got["docs"] != 0 {
		t.Fatalf("local commands dispatched fixer: %v", got)
	}
}

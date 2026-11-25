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
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func startPipelineWithTitle(t *testing.T, bin, repo, home string, v map[string]any, action, title string) map[string]any {
	t.Helper()
	request := map[string]any{"version": 1, "op": "pipeline", "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": action, "reason": "test adjustment", "title": title}
	b, _ := json.Marshal(request)
	return axi(t, bin, repo, home, string(b))
}

func TestFullPipelineReachesCIReadyWithLocalForge(t *testing.T) {
	t.Parallel()
	var bare, prBranch, prTitle, prBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
			var request struct {
				Head  string `json:"head"`
				Title string `json:"title"`
				Body  string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			prTitle, prBody = request.Title, request.Body
			prBranch = strings.TrimPrefix(request.Head, "acme:")
			head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+prBranch).Output()
			if err != nil {
				t.Errorf("read pushed head: %v", err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]string{"sha": strings.TrimSpace(string(head))}}) // Best-effort HTTP test response.
		case strings.HasSuffix(r.URL.Path, "/pulls/12") && r.Method == http.MethodGet:
			head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+prBranch).Output()
			if err != nil {
				t.Errorf("read PR head: %v", err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "head": map[string]string{"sha": strings.TrimSpace(string(head))}})
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			head := strings.Split(r.URL.Path, "/")
			sha := head[len(head)-2]
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "check_runs": []map[string]string{{"name": "fixture-ci", "status": "completed", "conclusion": "success", "head_sha": sha}}})
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
	started := startPipelineWithTitle(t, bin, repo, home, attached, "start", "feat(pipeline): publish reviewed candidate")
	if started["outcome"] != "ok" {
		t.Fatal(started)
	}
	done := waitPipeline(t, bin, repo, home)
	assertDeliveryReady(t, done)
	run := runOf(t, done)
	if prTitle != "feat(pipeline): publish reviewed candidate" || !strings.Contains(prBody, "## Summary") || !strings.Contains(prBody, "## Review evidence") || !strings.Contains(prBody, "No findings reported.") || !strings.Contains(prBody, "test: passed") || !strings.Contains(prBody, "lint: passed") || !strings.Contains(prBody, "Validated head: `"+run["candidate"].(string)+"`") {
		t.Fatalf("PR text omitted supplied title or delivery evidence: title=%q body=%q", prTitle, prBody)
	}
	if stageOf(t, done, "pr")["url"] != "https://example.invalid/acme/garden/pull/12" || stageOf(t, done, "pr")["number"] != float64(12) || stageOf(t, done, "ci")["head"] != run["candidate"] {
		t.Fatalf("PR identity or head evidence missing: %v", run["pipeline"])
	}
	if len(stageOf(t, done, "push")["commands"].([]any)) == 0 {
		t.Fatalf("delivery command-runner evidence was lost: %v", stageOf(t, done, "push"))
	}
	verifyDeliveryEvidence(t, home, bare, run)
}

func TestPipelineWithoutFullPresetRefusesWithoutCrashingDaemon(t *testing.T) {
	t.Parallel()
	bin, repo, home, prepared := prepareFixture(t)
	refused := mutate(t, bin, repo, home, prepared, "pipeline", "needs-full-preset", nil)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "prepare under the full preset first") {
		t.Fatalf("pipeline without full preset should be refused: %v", refused)
	}
	if inspected := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`); inspected["outcome"] != "ok" {
		t.Fatalf("daemon did not survive rejected pipeline request: %v", inspected)
	}
}

func TestPipelineStartPersistsDerivedTitleAndRejectsChanges(t *testing.T) {
	_, repo, home, prepared := pipelineFixture(t, func(root string) map[string]any {
		return map[string]any{"name": "Test", "email": "test@example.invalid", "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
	})
	stopDaemon(t, home)
	t.Setenv("GREENHOUSE_HOME", home)
	s, err := load(home)
	if err != nil {
		t.Fatal(err)
	}
	r := s.Runs[prepared["run_id"].(string)]
	request := Request{Version: 1, Op: "pipeline", Path: repo, RunID: r.ID, Expected: r.Token, Input: r.Input, Gate: currentGate(r), Action: "start", Reason: "test adjustment"}
	if got := handleRequest(s, request); got.Outcome != "ok" {
		t.Fatal(got)
	}
	if r.Pipeline.PullTitle != "feat: Change the sample" {
		t.Fatalf("derived title was not persisted at pipeline start: %q", r.Pipeline.PullTitle)
	}
	r.Status, r.Pipeline.Status = "needs-decision", "needs-decision"
	request.Expected, request.Input, request.Gate, request.Action, request.Title = r.Token, r.Input, currentGate(r), "change-title", "fix: use another title"
	if got := handleRequest(s, request); got.Outcome != "refused" || !strings.Contains(got.Error, "title is immutable") || r.Pipeline.PullTitle != "feat: Change the sample" {
		t.Fatalf("pipeline retry changed its title: %+v title=%q", got, r.Pipeline.PullTitle)
	}
}

func TestInterruptedPRMutationReconcilesWithoutDuplicateDelivery(t *testing.T) {
	t.Parallel()
	var bare, hookStarted, hookRelease, hookFinished, hookLog string
	var mu sync.Mutex
	exists, prHead, prTitle, prBody := false, "", "", ""
	var creates, updates atomic.Int32
	var logMu sync.Mutex
	var requestLog []string
	postStarted, releasePost, postFinished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logMu.Lock()
		requestLog = append(requestLog, r.Method+" "+r.URL.Path)
		logMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet:
			mu.Lock()
			found, head, title, body := exists, prHead, prTitle, prBody
			mu.Unlock()
			if !found {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "title": title, "body": body, "base": map[string]string{"ref": "main"}, "head": map[string]string{"sha": head}}})
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
			creates.Add(1)
			var request struct {
				Head  string `json:"head"`
				Title string `json:"title"`
				Body  string `json:"body"`
				Base  string `json:"base"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode PR request: %v", err)
				return
			}
			branch := strings.TrimPrefix(request.Head, "acme:")
			head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+branch).Output()
			if err != nil {
				t.Errorf("read pushed head: %v", err)
				return
			}
			mu.Lock()
			exists, prHead, prTitle, prBody = true, strings.TrimSpace(string(head)), request.Title, request.Body
			mu.Unlock()
			close(postStarted)
			<-releasePost
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "title": request.Title, "body": request.Body, "base": map[string]string{"ref": request.Base}, "head": map[string]string{"sha": strings.TrimSpace(string(head))}})
			close(postFinished)
		case r.URL.Path == "/repos/acme/garden/pulls/12" && r.Method == http.MethodPatch:
			updates.Add(1)
			var request struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			prTitle, prBody = request.Title, request.Body
			head := prHead
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]string{"sha": head}})
		case r.URL.Path == "/repos/acme/garden/pulls/12" && r.Method == http.MethodGet:
			mu.Lock()
			head := prHead
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "head": map[string]string{"sha": head}})
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			sha := strings.Split(r.URL.Path, "/")
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "check_runs": []map[string]string{{"name": "fixture-ci", "status": "completed", "conclusion": "success", "head_sha": sha[len(sha)-2]}}})
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
		hookStarted = filepath.Join(root, "push-hook-started")
		hookRelease = filepath.Join(root, "push-hook-release")
		hookFinished = filepath.Join(root, "push-hook-finished")
		hookLog = filepath.Join(root, "push-hook-log")
		hook := "#!/bin/sh\nprintf 'push\\n' >> " + hookLog + "\ntouch " + hookStarted + "\nwhile [ ! -e " + hookRelease + " ]; do sleep 0.01; done\ntouch " + hookFinished + "\n"
		if err := os.WriteFile(filepath.Join(bare, "hooks", "post-receive"), []byte(hook), 0700); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"name": "Test", "email": "test@example.invalid", "remote": bare, "github_owner": "acme", "github_repo": "garden", "pull_base": "main", "required_checks": []string{"fixture-ci"}, "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
	})
	started := mutate(t, bin, repo, home, attached, "pipeline", "start", nil)
	if started["outcome"] != "ok" {
		t.Fatal(started)
	}
	waitForFile := func(path string) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(path); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s", path)
	}
	killDaemon := func() {
		t.Helper()
		pidBytes, err := os.ReadFile(filepath.Join(home, "pid"))
		if err != nil {
			t.Fatal(err)
		}
		var pid int
		if err := json.Unmarshal(pidBytes, &pid); err != nil || pid <= 0 {
			t.Fatalf("invalid daemon pid %q: %v", pidBytes, err)
		}
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(30 * time.Second)
		for syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
			if time.Now().After(deadline) {
				_ = lock.Close() // Best-effort test lock cleanup.
				t.Fatal("interrupted daemon retained its lock")
			}
			time.Sleep(time.Millisecond)
		}
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
			t.Errorf("unlock test lock: %v", err)
		}
		_ = lock.Close() // Best-effort test lock cleanup.
	}
	waitForFile(hookStarted)
	killDaemon()
	if err := os.WriteFile(hookRelease, nil, 0600); err != nil {
		t.Fatal(err)
	}
	waitForFile(hookFinished)
	axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	select {
	case <-postStarted:
	case <-time.After(30 * time.Second):
		t.Fatal("PR creation did not start")
	}
	killDaemon()
	close(releasePost)
	select {
	case <-postFinished:
	case <-time.After(10 * time.Second):
		t.Fatal("fixture PR mutation did not finish")
	}
	state := waitPipeline(t, bin, repo, home)
	assertDeliveryReady(t, state)
	if creates.Load() != 1 || updates.Load() != 0 {
		t.Fatalf("PR effects duplicated or not reconciled: creates=%d updates=%d", creates.Load(), updates.Load())
	}
	mu.Lock()
	gotTitle, gotBody := prTitle, prBody
	mu.Unlock()
	if gotTitle != "feat: Change the sample" || !strings.Contains(gotBody, "Validated head: `"+runOf(t, state)["candidate"].(string)+"`") || !strings.Contains(gotBody, "No findings reported.") {
		t.Fatalf("reconciled PR text is incomplete: title=%q body=%q", gotTitle, gotBody)
	}
	pushes, err := os.ReadFile(hookLog)
	if err != nil || strings.Count(string(pushes), "push\n") != 1 {
		t.Fatalf("push mutation count: log=%q err=%v", pushes, err)
	}
	logMu.Lock()
	prMutations := 0
	for _, request := range requestLog {
		if request == "POST /repos/acme/garden/pulls" || request == "PATCH /repos/acme/garden/pulls/12" {
			prMutations++
		}
	}
	logMu.Unlock()
	if prMutations != 1 {
		t.Fatalf("fake forge PR mutation log: %v", requestLog)
	}
	if stageOf(t, state, "pr")["url"] != "https://example.invalid/acme/garden/pull/12" || stageOf(t, state, "ci")["head"] != runOf(t, state)["candidate"] {
		t.Fatalf("delivery identity was not reconciled: %v", runOf(t, state)["pipeline"])
	}
}

func TestFullPipelineReportsMissingAndStaleChecksThroughAXI(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, payload, want string }{
		{"missing", `{"total_count":0,"check_runs":[]}`, "ci-ready"},
		{"stale", `{"total_count":1,"check_runs":[{"name":"fixture-ci","status":"completed","conclusion":"success","head_sha":"stale-head"}]}`, "needs-decision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bare, branch string
			var checkCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet:
					_, _ = w.Write([]byte(`[]`))
				case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
					var request struct {
						Head string `json:"head"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					branch = strings.TrimPrefix(request.Head, "acme:")
					head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+branch).Output()
					if err != nil {
						t.Errorf("read pushed head: %v", err)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]string{"sha": strings.TrimSpace(string(head))}}) // Best-effort HTTP test response.
				case r.URL.Path == "/repos/acme/garden/pulls/12":
					head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+branch).Output()
					if err != nil {
						t.Errorf("read PR head: %v", err)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "head": map[string]string{"sha": strings.TrimSpace(string(head))}})
				case strings.HasSuffix(r.URL.Path, "/check-runs"):
					if tc.name == "missing" && checkCalls.Add(1) > 1 {
						sha := strings.Split(r.URL.Path, "/")
						_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "check_runs": []map[string]string{{"name": "fixture-ci", "status": "completed", "conclusion": "success", "head_sha": sha[len(sha)-2]}}})
						return
					}
					_, _ = w.Write([]byte(tc.payload))
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
			if runOf(t, state)["status"] != tc.want {
				t.Fatalf("unexpected AXI delivery status: %v", runOf(t, state)["pipeline"])
			}
			if tc.name == "missing" && (checkCalls.Load() < 2 || stageOf(t, state, "ci")["head"] != runOf(t, state)["candidate"]) {
				t.Fatalf("missing check was not polled to exact-head readiness: calls=%d pipeline=%v", checkCalls.Load(), runOf(t, state)["pipeline"])
			}
			if tc.name == "stale" && stageOf(t, state, "ci")["reason"] != "stale check: fixture-ci" {
				t.Fatalf("stale check not reported through AXI: %v", runOf(t, state)["pipeline"])
			}
		})
	}
}

func TestFullPipelineWaitsForRequiredCheckToAppear(t *testing.T) {
	t.Parallel()
	var bare, branch string
	var checks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
			var request struct {
				Head string `json:"head"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			branch = strings.TrimPrefix(request.Head, "acme:")
			head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+branch).Output()
			if err != nil {
				t.Errorf("read pushed head: %v", err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]string{"sha": strings.TrimSpace(string(head))}}) // Best-effort HTTP test response.
		case r.URL.Path == "/repos/acme/garden/pulls/12":
			head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+branch).Output()
			if err != nil {
				t.Errorf("read PR head: %v", err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "head": map[string]string{"sha": strings.TrimSpace(string(head))}})
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			sha := strings.Split(r.URL.Path, "/")
			if checks.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "check_runs": []map[string]string{{"name": "optional-lint", "status": "completed", "conclusion": "success", "head_sha": sha[len(sha)-2]}}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "check_runs": []map[string]string{{"name": "fixture-ci", "status": "completed", "conclusion": "success", "head_sha": sha[len(sha)-2]}}})
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
	assertDeliveryReady(t, state)
	if checks.Load() < 2 {
		t.Fatalf("pipeline passed without polling for the required check: calls=%d", checks.Load())
	}
}

func TestFullPipelineTransportExhaustionDoesNotDispatchFixer(t *testing.T) {
	t.Parallel()
	var bare string
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet {
			requests++
			_, _ = w.Write([]byte(`[]`))
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
	t.Parallel()
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
				_, _ = w.Write([]byte(`[]`))
				return
			}
			branchName = r.URL.Query().Get("head")
			_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]any{"sha": currentHead(branchName)}}})
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
			prCreates++
			var req struct {
				Head string `json:"head"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
				return
			}
			branchName = req.Head
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]any{"sha": currentHead(branchName)}})
		case r.Method == http.MethodPatch:
			prUpdates++
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "head": map[string]any{"sha": currentHead(branchName)}})
		case strings.HasSuffix(r.URL.Path, "/pulls/12") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 12, "head": map[string]string{"sha": currentHead(branchName)}})
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			ciCalls++
			split := strings.Split(r.URL.Path, "/")
			sha := split[len(split)-2]
			conclusion := "success"
			if ciCalls == 1 {
				conclusion = "failure"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 2, "check_runs": []map[string]string{{"name": "fixture-ci", "status": "completed", "conclusion": conclusion, "head_sha": sha}, {"name": "optional-lint", "status": "completed", "conclusion": "success", "head_sha": sha}}})
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
	if err := os.WriteFile(filepath.Join(root, "fix-content"), []byte("repaired\n"), 0600); err != nil {
		t.Fatal(err)
	}
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
			run := &Run{ID: "run", Input: "input", Token: "token", Candidate: "candidate", Status: "running", Configs: []Snapshot{{ID: "config", Settings: settings}}, Pipeline: newPipeline(CommandPlan{}, map[string]string{}, "per-user")}
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
			if got.Status != "running" || got.Pipeline.Status != "running" || got.Pipeline.Active != nil || got.Pipeline.stage(name).Status != "pending" || got.Pipeline.stage(name).Inputs != "" || got.Pipeline.stage(name).Number != 42 || got.Pipeline.stage(name).Runs != 2 {
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

var forgeAPIs sync.Map

func setForgeAPI(t *testing.T, value string) {
	t.Helper()
	forgeAPIs.Store(t, value)
	t.Cleanup(func() { forgeAPIs.Delete(t) })
}

func forgeEnv(t *testing.T) []string {
	if value, ok := forgeAPIs.Load(t); ok {
		return []string{"GREENHOUSE_GITHUB_API=" + value.(string)}
	}
	return nil
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

func TestLinkedWorktreePipelinesDeliverConcurrently(t *testing.T) {
	t.Parallel()
	var bare string
	var mu sync.Mutex
	branches := map[int]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		remoteHead := func(branch string) string {
			head, err := exec.Command("git", "--git-dir", bare, "rev-parse", "refs/heads/"+branch).Output()
			if err != nil {
				t.Errorf("read pushed head of %s: %v", branch, err)
			}
			return strings.TrimSpace(string(head))
		}
		switch {
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
			var request struct {
				Head string `json:"head"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			branch := strings.TrimPrefix(request.Head, "acme:")
			mu.Lock()
			number := 20 + len(branches)
			branches[number] = branch
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"number": number, "html_url": fmt.Sprintf("https://example.invalid/acme/garden/pull/%d", number), "head": map[string]string{"sha": remoteHead(branch)}})
		case strings.HasPrefix(r.URL.Path, "/repos/acme/garden/pulls/") && r.Method == http.MethodGet:
			var number int
			_, _ = fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/repos/acme/garden/pulls/"), "%d", &number) // Path is controlled by this test.
			mu.Lock()
			branch := branches[number]
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"number": number, "head": map[string]string{"sha": remoteHead(branch)}})
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			parts := strings.Split(r.URL.Path, "/")
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "check_runs": []map[string]string{{"name": "fixture-ci", "status": "completed", "conclusion": "success", "head_sha": parts[len(parts)-2]}}})
		default:
			t.Errorf("unexpected forge request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	setForgeAPI(t, server.URL)
	bin, repo, home, first := pipelineFixture(t, func(root string) map[string]any {
		bare = filepath.Join(root, "remote.git")
		if err := exec.Command("git", "init", "--bare", bare).Run(); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"name": "Test", "email": "test@example.invalid", "remote": bare, "github_owner": "acme", "github_repo": "garden", "pull_base": "main", "required_checks": []string{"fixture-ci"}, "test": [][]string{gateArgv(root, "test")}, "lint": [][]string{gateArgv(root, "lint")}, "not_applicable": map[string]string{"docs": "fixture has no docs"}}
	})
	root := filepath.Dir(home)
	target := runOf(t, first)["target"].(string)
	prepare := func(dir, name string) map[string]any {
		v := axi(t, bin, dir, home, `{"version":1,"op":"attach"}`)
		v = mutate(t, bin, dir, home, v, "apply", "full-"+name, map[string]any{"mode": "auto", "preset": "full", "limit": 6, "rules": []string{"unslop"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"})
		q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": v["gate_id"], "action_id": "prepare-" + name, "intent": "Change the sample", "target": target}
		b, _ := json.Marshal(q)
		if v = axi(t, bin, dir, home, string(b)); v["outcome"] != "ok" {
			t.Fatal(v)
		}
		return v
	}
	linked := filepath.Join(root, "linked")
	git(t, repo, "worktree", "add", "-q", "-b", "feature", linked, "HEAD")
	if err := os.WriteFile(filepath.Join(linked, "sample.txt"), []byte("feature sample\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, linked, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qam", "feature candidate")
	second := prepare(linked, "linked")
	same := filepath.Join(root, "same-branch")
	git(t, repo, "worktree", "add", "-q", "-f", same, "main")
	sameBranch := prepare(same, "same")

	blockTest := filepath.Join(root, "block-test")
	if err := os.WriteFile(blockTest, nil, 0600); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(blockTest) }()
	if started := mutate(t, bin, repo, home, first, "pipeline", "start", nil); started["outcome"] != "ok" {
		t.Fatal(started)
	}
	deadline := testWaitDeadline(t)
	for time.Now().Before(deadline) && countLines(t, filepath.Join(root, "commands.log"))["test"] == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if started := mutate(t, bin, linked, home, second, "pipeline", "start-linked", nil); started["outcome"] != "ok" {
		t.Fatalf("linked worktree on another branch refused while the first pipeline runs: %v", started)
	}
	refused := mutate(t, bin, same, home, sameBranch, "pipeline", "start-same", nil)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "branch main") {
		t.Fatalf("second run on the same branch was not refused with a clear reason: %v", refused)
	}
	running := runOf(t, axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`))
	if running["status"] != "running" {
		t.Fatal("first pipeline finished before the second started; runs did not overlap")
	}
	active := stageOf(t, map[string]any{"run": running}, "test")
	if _, err := time.Parse(time.RFC3339, active["started_at"].(string)); err != nil || active["finished_at"] != nil {
		t.Fatalf("running stage timestamps are invalid: %v", active)
	}
	if err := os.Remove(blockTest); err != nil {
		t.Fatal(err)
	}

	firstDone, secondDone := waitPipeline(t, bin, repo, home), waitPipeline(t, bin, linked, home)
	for _, done := range []map[string]any{firstDone, secondDone} {
		assertDeliveryReady(t, done)
		verifyDeliveryEvidence(t, home, bare, runOf(t, done))
	}
	if stageOf(t, firstDone, "pr")["number"] == stageOf(t, secondDone, "pr")["number"] {
		t.Fatalf("both pipelines reported one PR: %v", stageOf(t, firstDone, "pr"))
	}
	if got := countLines(t, filepath.Join(root, "commands.log")); got["test"] != 2 || got["lint"] != 2 {
		t.Fatalf("each pipeline must run its gates once: %v", got)
	}
}

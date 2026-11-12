package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeRemoteRejectsCredentialAndAliasInputs(t *testing.T) {
	for _, remote := range []string{"origin", "https://user:secret@example.invalid/acme/garden.git", "https://example.invalid/garden.git?token=secret"} {
		if safeRemote(remote) {
			t.Errorf("accepted unsafe remote %q", remote)
		}
	}
	for _, remote := range []string{"git@github.com:acme/garden.git", "/srv/git/garden.git"} {
		if !safeRemote(remote) {
			t.Errorf("rejected safe remote %q", remote)
		}
	}
}

func TestGitHubForgeReusesExistingPullRequest(t *testing.T) {
	creates, updates := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/garden/pulls/7" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"number":7,"head":{"sha":"head-a"}}`))
			return
		}
		if r.URL.Path == "/repos/acme/garden/commits/head-a/check-runs" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"check_runs":[{"name":"trusted-ci","status":"completed","conclusion":"success","head_sha":"head-a"},{"name":"optional-lint","status":"completed","conclusion":"success","head_sha":"head-a"}]}`))
			return
		}
		if r.URL.Path == "/repos/acme/garden/pulls/7" && r.Method == http.MethodPatch {
			updates++
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["title"] != "title" || body["base"] != "main" {
				t.Errorf("unexpected PR update: %v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"number":7,"html_url":"https://example.invalid/acme/garden/pull/7","head":{"sha":"head-a"}}`))
			return
		}
		if r.URL.Path != "/repos/acme/garden/pulls" {
			t.Fatalf("unexpected forge path %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"number":7,"html_url":"https://example.invalid/acme/garden/pull/7","head":{"sha":"head-a"}}]`))
		case http.MethodPost:
			creates++
			t.Fatal("existing PR should not be recreated")
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	forge := GitHubForge{BaseURL: server.URL, Client: server.Client()}
	pr, err := forge.EnsurePullRequest(context.Background(), "acme", "garden", "topic", "main", "head-a", "title", "body")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 7 || pr.URL != "https://example.invalid/acme/garden/pull/7" || creates != 0 || updates != 1 {
		t.Fatalf("PR was not reconciled: %+v creates=%d", pr, creates)
	}
	passed, reason, err := forge.Checks(context.Background(), "acme", "garden", "head-a", 7, []string{"trusted-ci"})
	if err != nil || !passed || reason != "all required checks passed for exact head" {
		t.Fatalf("exact-head checks not accepted: passed=%t reason=%q err=%v", passed, reason, err)
	}
}

func TestGitHubForgeReconcilesAlreadyExactPullRequestWithoutMutation(t *testing.T) {
	var mutations int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/repos/acme/garden/pulls" {
			json.NewEncoder(w).Encode([]map[string]any{{"number": 12, "html_url": "https://example.invalid/acme/garden/pull/12", "title": "title", "body": "body", "base": map[string]string{"ref": "main"}, "head": map[string]string{"sha": "head-a"}}})
			return
		}
		if r.Method == http.MethodPatch || r.Method == http.MethodPost {
			mutations++
		}
		http.Error(w, "unexpected request", http.StatusNotFound)
	}))
	defer server.Close()
	pr, err := (GitHubForge{BaseURL: server.URL, Client: server.Client()}).EnsurePullRequest(context.Background(), "acme", "garden", "topic", "main", "head-a", "title", "body")
	if err != nil || pr.Number != 12 || pr.Head != "head-a" || mutations != 0 {
		t.Fatalf("exact PR was mutated or not reconciled: pr=%+v mutations=%d err=%v", pr, mutations, err)
	}
}

func TestGitHubForgeMissingAndStaleChecksDoNotPass(t *testing.T) {
	for _, tc := range []struct{ name, payload, want string }{
		{"missing", `{"check_runs":[]}`, "missing required check: trusted-ci"},
		{"stale", `{"check_runs":[{"name":"trusted-ci","status":"completed","conclusion":"success","head_sha":"old-head"}]}`, "stale check: trusted-ci"},
		{"unknown", `{"check_runs":[{"name":"trusted-ci","status":"completed","head_sha":"head-a"}]}`, "checks pending: trusted-ci"},
		{"ancillary-only", `{"check_runs":[{"name":"optional-ci","status":"completed","conclusion":"success","head_sha":"head-a"}]}`, "missing required check: trusted-ci"},
		{"pr-head-moved", `{"check_runs":[{"name":"trusted-ci","status":"completed","conclusion":"success","head_sha":"head-a"}]}`, "pull request head moved from authorized candidate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/repos/acme/garden/pulls/4" {
					sha := "head-a"
					if tc.name == "pr-head-moved" {
						sha = "head-b"
					}
					w.Write([]byte(`{"number":4,"head":{"sha":"` + sha + `"}}`))
					return
				}
				w.Write([]byte(tc.payload))
			}))
			defer server.Close()
			passed, reason, err := (GitHubForge{BaseURL: server.URL, Client: server.Client()}).Checks(context.Background(), "acme", "garden", "head-a", 4, []string{"trusted-ci"})
			if err != nil || passed || reason != tc.want {
				t.Fatalf("unexpected result: %t %q %v", passed, reason, err)
			}
		})
	}
}

func TestDeliveryPipelinePushesCreatesPRAndWaitsForExactChecks(t *testing.T) {
	root := t.TempDir()
	repo, bare := filepath.Join(root, "repo"), filepath.Join(root, "remote.git")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-b", "greenhouse/test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("candidate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "README.md")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	if err := exec.Command("git", "init", "--bare", bare).Run(); err != nil {
		t.Fatal(err)
	}
	id, err := resolve(repo)
	if err != nil {
		t.Fatal(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	gitDigest, err := executableDigest(gitPath)
	if err != nil {
		t.Fatal(err)
	}
	p := Pipeline{Plan: CommandPlan{Name: "Test", Email: "test@example.invalid", Remote: bare, GitHubOwner: "acme", GitHubRepo: "garden", PullBase: "main", RequiredChecks: []string{"ci"}}, CommandDigests: map[string]string{gitPath: gitDigest}, Worktree: repo, Ref: "refs/heads/greenhouse/test"}
	p.Stages = newPipeline(p.Plan, p.CommandDigests).Stages
	var prCreated int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodGet:
			w.Write([]byte(`[]`))
		case r.URL.Path == "/repos/acme/garden/pulls/8" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{"number": 8, "head": map[string]string{"sha": id.Head}})
		case r.URL.Path == "/repos/acme/garden/pulls" && r.Method == http.MethodPost:
			prCreated++
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["head"] != "greenhouse/test" || body["base"] != "main" {
				t.Errorf("unexpected PR request: %v", body)
			}
			json.NewEncoder(w).Encode(map[string]any{"number": 8, "html_url": "https://example.invalid/acme/garden/pull/8", "head": map[string]string{"sha": id.Head}})
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			w.Write([]byte(`{"check_runs":[{"name":"ci","status":"completed","conclusion":"success","head_sha":"` + id.Head + `"}]}`))
		default:
			t.Errorf("unexpected forge request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	old := os.Getenv("GREENHOUSE_GITHUB_API")
	os.Setenv("GREENHOUSE_GITHUB_API", server.URL)
	defer os.Setenv("GREENHOUSE_GITHUB_API", old)

	push, head, _ := runDeliveryStage(context.Background(), p, "push", id.Head)
	if push.Status != "passed" || head != id.Head || strings.Contains(strings.Join(commandNames(push.Commands), " "), "push") == false {
		t.Fatalf("push did not pass: %+v", push)
	}
	push, _, _ = runDeliveryStage(context.Background(), p, "push", id.Head)
	if push.Status != "passed" || !strings.Contains(push.Reason, "reconciled") {
		t.Fatalf("repeat push was not reconciled: %+v", push)
	}
	pr, _, url := runDeliveryStage(context.Background(), p, "pr", id.Head)
	if pr.Status != "passed" || url != "https://example.invalid/acme/garden/pull/8" || prCreated != 1 {
		t.Fatalf("PR stage failed: %+v creates=%d", pr, prCreated)
	}
	p.stage("pr").Number, p.stage("pr").Head = 8, id.Head
	ci, _, _ := runDeliveryStage(context.Background(), p, "ci", id.Head)
	if ci.Status != "passed" {
		t.Fatalf("CI stage failed: %+v", ci)
	}
}

func commandNames(results []CheckResult) []string {
	var names []string
	for _, result := range results {
		if len(result.Args) > 0 {
			names = append(names, strings.Join(result.Args, " "))
		}
	}
	return names
}

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

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
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
	id, err := repository.Resolve(repo)
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

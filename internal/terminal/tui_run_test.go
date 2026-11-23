package terminal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

func TestTargetDisplayNamesResolvedBranch(t *testing.T) {
	repo := intentRepo(t)
	commit, err := exec.Command("git", "-C", repo, "rev-parse", "main").Output()
	if err != nil {
		t.Fatal(err)
	}
	short := strings.TrimSpace(string(commit))[:7]
	if got, want := targetDisplay(repo, strings.TrimSpace(string(commit)), "main"), "main @ "+short; got != want {
		t.Fatalf("target display = %q, want %q", got, want)
	}
}

func TestInferCommitIntentUsesMessagesAndBranch(t *testing.T) {
	repo := intentRepo(t)
	intent, source, err := inferCommitIntent(repo, "main", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if intent != "Improve file handling" || source != "commit messages + branch feature" {
		t.Fatalf("intent=%q source=%q", intent, source)
	}
}

func TestInferCommitIntentUsesSignedInGHCLI(t *testing.T) {
	repo := intentRepo(t)
	gitCommand(t, repo, "remote", "add", "origin", "git@github.com:acme/garden.git")
	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n[ \"$*\" = \"pr view --json body,state\" ] || exit 2\nprintf '%s\\n' '{\"body\":\"Plan the file handling changes.\",\"state\":\"OPEN\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+filepath.Dir(gitPath)+":"+os.Getenv("PATH"))
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	intent, source, err := inferCommitIntent(repo, "main", "HEAD")
	if err != nil || intent != "Plan the file handling changes." || source != "open pull request body" {
		t.Fatalf("intent=%q source=%q error=%v", intent, source, err)
	}
}

func TestInferCommitIntentPrefersOpenPullRequestBody(t *testing.T) {
	repo := intentRepo(t)
	gitCommand(t, repo, "remote", "add", "origin", "git@github.com:acme/garden.git")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "head=acme%3Afeature") {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("authorization = %q, want token auth", got)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 12, "body": "Improve file handling and add validation."}})
	}))
	defer server.Close()
	gh := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(gh)+":"+os.Getenv("PATH"))
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GREENHOUSE_GITHUB_API", server.URL)
	intent, source, err := inferCommitIntent(repo, "main", "HEAD")
	if err != nil || intent != "Improve file handling and add validation." || source != "PR #12 body" {
		t.Fatalf("intent=%q source=%q error=%v", intent, source, err)
	}
}

func intentRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitCommand(t, repo, "init", "-q", "-b", "main")
	gitCommand(t, repo, "config", "user.name", "Test")
	gitCommand(t, repo, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "add", "file.txt")
	gitCommand(t, repo, "commit", "-qm", "Base")
	gitCommand(t, repo, "checkout", "-qb", "feature")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "commit", "-qam", "Improve file handling")
	return repo
}

func gitCommand(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func TestSetupStepsNameTheResolvedTarget(t *testing.T) {
	repo := intentRepo(t)
	commit, err := exec.Command("git", "-C", repo, "rev-parse", "main").Output()
	if err != nil {
		t.Fatal(err)
	}
	v := ready(tuiState("running"))
	v.Run.Submitted.Worktree = repo
	v.Run.Target = strings.TrimSpace(string(commit))
	steps := setupSteps(v, []string{"claude"}, "main")
	if want := "main @ " + v.Run.Target[:7]; steps[2].Text != want {
		t.Fatalf("target step = %q, want %q", steps[2].Text, want)
	}
}

func TestReviewStepFailsOnlyWhenAFindingBlocks(t *testing.T) {
	for _, tc := range []struct {
		priority string
		state    string
		text     string
	}{
		{"P2", stepDone, "1 finding (none blocking)"},
		{"P1", stepFailed, "1 outstanding finding"},
	} {
		v := tuiState("outstanding-findings")
		v.Run.Findings = []engine.Finding{{ID: "f1", Priority: tc.priority, File: "a.go", Start: 1, Explanation: "x", Action: "fix"}}
		got := reviewStep(v)
		if got.State != tc.state || !strings.HasPrefix(got.Text, tc.text) {
			t.Fatalf("%s: state=%v text=%q", tc.priority, got.State, got.Text)
		}
	}
}

func TestPipelinePlanLineHiddenForOnePassWithoutPlan(t *testing.T) {
	v := tuiState("outstanding-findings")
	v.Repository = &engine.RepositoryStatus{PipelineStatus: "absent"}
	if showPipelinePlan(v) {
		t.Fatal("absent plan shown for a one-pass run")
	}
	v.Repository.PipelineStatus = "trusted"
	if !showPipelinePlan(v) {
		t.Fatal("present plan hidden")
	}
}

func TestInferCommitIntentRejectsOptionLikeTarget(t *testing.T) {
	repo := intentRepo(t)
	out := filepath.Join(t.TempDir(), "injected")
	if _, _, err := inferCommitIntent(repo, "--output="+out, "HEAD"); err == nil {
		t.Fatal("option-like target was accepted")
	}
	if matches, _ := filepath.Glob(out + "*"); len(matches) > 0 {
		t.Fatalf("git ran the target as an option and wrote %v", matches)
	}
}

func TestInferCommitIntentIgnoresClosedPullRequestFromGHCLI(t *testing.T) {
	repo := intentRepo(t)
	gitCommand(t, repo, "remote", "add", "origin", "git@github.com:acme/garden.git")
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '{\"body\":\"Old merged work.\",\"state\":\"MERGED\"}'\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+filepath.Dir(gitPath))
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	intent, source, err := inferCommitIntent(repo, "main", "HEAD")
	if err != nil || intent != "Improve file handling" || source != "commit messages + branch feature" {
		t.Fatalf("intent=%q source=%q error=%v", intent, source, err)
	}
}

func TestGitHubRepositoryParsesRemoteForms(t *testing.T) {
	for _, remote := range []string{"git@github.com:acme/garden.git", "https://github.com/acme/garden.git", "ssh://git@github.com/acme/garden.git"} {
		if owner, repo, ok := githubRepository(remote); !ok || owner != "acme" || repo != "garden" {
			t.Errorf("%s parsed as %q/%q ok=%t", remote, owner, repo, ok)
		}
	}
	if _, _, ok := githubRepository("https://token@github.com/acme/garden.git"); ok {
		t.Error("credentialed https remote was accepted")
	}
}

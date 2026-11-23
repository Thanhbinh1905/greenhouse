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
)

func TestInferCommitIntentUsesMessagesAndBranch(t *testing.T) {
	repo := intentRepo(t)
	intent, source, err := inferCommitIntent(repo, "main", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if intent != "Commit messages and branch feature: Improve file handling" || source != "commit messages + branch feature" {
		t.Fatalf("intent=%q source=%q", intent, source)
	}
}

func TestInferCommitIntentPrefersOpenPullRequestBody(t *testing.T) {
	repo := intentRepo(t)
	gitCommand(t, repo, "remote", "add", "origin", "git@github.com:acme/garden.git")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "head=acme%3Afeature") {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 12, "body": "Improve file handling and add validation."}})
	}))
	defer server.Close()
	t.Setenv("GREENHOUSE_GITHUB_API", server.URL)
	intent, source, err := inferCommitIntent(repo, "main", "HEAD")
	if err != nil || intent != "PR #12 body: Improve file handling and add validation." || source != "PR #12 body" {
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

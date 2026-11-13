package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

func TestHostFindingIdentityAcrossRounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, next string
		same       bool
	}{
		{"shift and reword", "unrelated\n\n  flagged   code  \n", true},
		{"edit flagged code", "different code\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, repo, home := fixture(t)
			target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
			commit := func(content string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				git(t, repo, "add", "sample.txt")
				git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
			}
			commit("flagged code\n")
			v := axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"fake-host","id":"task-7"}}`)
			installFake(t, home, "")
			fake := filepath.Join(filepath.Dir(home), "fake-pi")
			script, err := os.ReadFile(fake)
			if err != nil {
				t.Fatal(err)
			}
			script = []byte(strings.Replace(string(script), "print(json.dumps({'type':'message_end'", `lines=b['Files'][0]['Lines'].splitlines()
n=int(lines[-1].split(':',1)[0])
v['findings'][0].update(start=n,end=n,explanation='Explanation at line '+str(n))
print(json.dumps({'type':'message_end'`, 1))
			if err := os.WriteFile(fake, script, 0700); err != nil {
				t.Fatal(err)
			}
			review := func(round int) string {
				t.Helper()
				suffix := fmt.Sprintf("-%d", round)
				v = hostPrepare(t, bin, repo, home, v, target, suffix)
				if started := hostCall(t, bin, repo, home, v, "review", "review"+suffix, map[string]any{"grant": "round" + suffix}); started["outcome"] != "ok" {
					t.Fatal(started)
				}
				v = hostSettle(t, bin, repo, home)
				result := v["host_result"].(map[string]any)
				findings, _ := result["findings"].([]any)
				if result["verdict"] != "findings" || result["current"] != true || len(findings) != 1 {
					t.Fatalf("review result: %v", result)
				}
				return findings[0].(map[string]any)["id"].(string)
			}
			first := review(1)
			commit(tc.next)
			v = hostCall(t, bin, repo, home, v, "new", "next-round", nil)
			if v["outcome"] != "ok" {
				t.Fatal(v)
			}
			second := review(2)
			if (first == second) != tc.same {
				t.Fatalf("finding IDs: first=%s second=%s; want unchanged=%t", first, second, tc.same)
			}
		})
	}
}

func TestFindingIdentityNormalizationAndDuplicates(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("  flagged\tcode \n\n next   line\nflagged code\nnext line\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("uncommitted replacement\n"), 0600); err != nil {
		t.Fatal(err)
	}
	x := &Execution{FindingIdentity: "code-v1", Source: repository.Identity{Worktree: repo}, Rules: InputIdentity{Candidate: head}, Paths: []string{"sample.txt"}, Diff: "--- /dev/null\n+++ b/sample.txt\n@@ -0,0 +1,5 @@\n"}
	first := Finding{Priority: "P2", Origin: "correctness", File: "sample.txt", Start: 1, End: 3, Explanation: "First explanation", Action: "Fix", Head: head}
	second := first
	second.Start, second.End, second.Explanation, second.Priority = 4, 5, "Reworded explanation", "P1"
	parse := func(findings ...Finding) (*Verdict, error) {
		body, err := json.Marshal(Verdict{Head: head, Verdict: "findings", Coverage: x.Paths, Findings: findings})
		if err != nil {
			t.Fatal(err)
		}
		return parseReview(verdictEvents(t, string(body)), x)
	}
	v, err := parse(first, second)
	if err != nil {
		t.Fatal(err)
	}
	want := Digest([]any{"correctness", "sample.txt", "", "flagged code\nnext line"})
	if len(v.Findings) != 1 || v.Findings[0].ID != want || v.Findings[0].Explanation != first.Explanation || v.Findings[0].Priority != first.Priority {
		t.Fatalf("duplicate findings: %+v; want first finding with ID %s", v.Findings, want)
	}
	second.End = 6
	if _, err := parse(first, second); err == nil {
		t.Fatal("accepted out-of-file duplicate range")
	}
	x.FindingIdentity = ""
	v, err = parse(first)
	if err != nil {
		t.Fatal(err)
	}
	legacy := Digest([]any{first.Origin, first.File, first.Start, first.End, first.Rule, first.Explanation})
	if v.Findings[0].ID != legacy {
		t.Fatalf("legacy execution ID changed: got %s want %s", v.Findings[0].ID, legacy)
	}
	x.Result = v
	saved, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	var restored Execution
	if err := json.Unmarshal(saved, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.FindingIdentity != "" || restored.Result.Findings[0].ID != legacy || strings.Contains(string(saved), "finding_identity") {
		t.Fatalf("legacy record changed: %s", saved)
	}
}

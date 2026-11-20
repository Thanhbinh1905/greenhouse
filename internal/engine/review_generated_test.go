package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cleanFakeReview = `print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps({'head':b['Head'],'verdict':'clean','coverage':b['Paths'],'findings':[]})}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
sys.exit(0)`

func writeFixtureFile(t *testing.T, repo, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, path)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func largeMigration() string {
	var b strings.Builder
	for i := 0; b.Len() < 300*1024; i++ {
		fmt.Fprintf(&b, "INSERT INTO baseline (id, name) VALUES (%d, 'row %d');\n", i, i)
	}
	return b.String()
}

type capturedReview struct {
	Input struct {
		Paths     []string
		Generated []struct {
			Path       string
			Added      int
			Removed    int
			Generators []string
		}
		Uncovered []struct{ Path, Reason string }
		Files     []reviewFile
		Diff      string
	}
	Raw string
}

func readCapturedReview(t *testing.T, home string) capturedReview {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c capturedReview
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func reviewFileLines(c capturedReview, path string) (string, bool) {
	for _, file := range c.Input.Files {
		if file.Path == path {
			return file.Lines, true
		}
	}
	return "", false
}

func TestReviewTrustedGeneratedFile(t *testing.T) {
	bin, repo, home := fixture(t)
	writeFixtureFile(t, repo, ".gitattributes", "migrations/*.sql linguist-generated greenhouse-generator=tools/gen.go\n")
	writeFixtureFile(t, repo, "tools/gen.go", "package tools\n\nfunc Generate() {}\n")
	target := commitReviewFixture(t, repo, "declare generated migrations")
	migration := largeMigration()
	writeFixtureFile(t, repo, "migrations/baseline.sql", migration)
	writeFixtureFile(t, repo, "sample.txt", "review me\n")
	commitReviewFixture(t, repo, "squash migrations")

	v := prepareReviewAdmission(t, bin, repo, home, target)
	installFake(t, home, cleanFakeReview)
	v = mutate(t, bin, repo, home, v, "review", "generated", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("trusted generated file refused: %v", v["error"])
	}
	run := runOf(t, waitReview(t, bin, repo, home))
	review := run["review"].(map[string]any)
	if run["status"] != "clean-review" || review["result"].(map[string]any)["verdict"] != "clean" {
		t.Fatalf("trusted generated file blocked a clean verdict: %v", review)
	}
	lines := strings.Count(migration, "\n")
	want := fmt.Sprintf(`[{"added":%d,"generators":["tools/gen.go"],"path":"migrations/baseline.sql","removed":0}]`, lines)
	if got := mustJSON(t, review["generated"]); got != want {
		t.Fatalf("coverage does not record the generated path:\n got %s\nwant %s", got, want)
	}

	c := readCapturedReview(t, home)
	if len(c.Raw) > 64*1024 {
		t.Fatalf("generated content sent in full: %d bytes", len(c.Raw))
	}
	if strings.Contains(c.Input.Diff, "VALUES (5000,") || !strings.Contains(c.Input.Diff, "diff --git a/migrations/baseline.sql b/migrations/baseline.sql") || !strings.Contains(c.Input.Diff, "+review me") {
		t.Fatalf("diff must keep the generated header and drop its hunks: %d bytes", len(c.Input.Diff))
	}
	excerpt, ok := reviewFileLines(c, "migrations/baseline.sql")
	if !ok || len(excerpt) > 5*1024 || !strings.HasPrefix(excerpt, "1: INSERT INTO baseline (id, name) VALUES (0, 'row 0');\n") || !strings.HasSuffix(excerpt, "...\n") {
		t.Fatalf("generated excerpt missing or unbounded: %d bytes", len(excerpt))
	}
	if generator, ok := reviewFileLines(c, "tools/gen.go"); !ok || !strings.Contains(generator, "3: func Generate() {}\n") {
		t.Fatalf("producing file missing from review input: %v", c.Input.Files)
	}
	if len(c.Input.Generated) != 1 || c.Input.Generated[0].Added != lines || strings.Join(c.Input.Generated[0].Generators, ",") != "tools/gen.go" {
		t.Fatalf("reviewer input lacks the generated diffstat: %+v", c.Input.Generated)
	}
	if strings.Join(c.Input.Paths, ",") != "migrations/baseline.sql,sample.txt" {
		t.Fatalf("generated path must still require coverage: %v", c.Input.Paths)
	}
}

func assertPartialReview(t *testing.T, v map[string]any, home, path, covered string) {
	t.Helper()
	result := v["host_result"].(map[string]any)
	reason := "hunk windows exceed 65536 bytes"
	if result["status"] != "partial-review" || result["verdict"] != "partial" || result["current"] != true {
		t.Fatalf("oversized file must give a non-clean partial outcome: %v", result)
	}
	uncovered := mustJSON(t, result["uncovered"])
	if !strings.HasPrefix(uncovered, `[{"path":"`+path+`","reason":"`+reason) || strings.Count(uncovered, `"path"`) != 1 {
		t.Fatalf("host result must name the uncovered path and reason: %s", uncovered)
	}
	if got := mustJSON(t, runOf(t, v)["review"].(map[string]any)["uncovered"]); got != uncovered {
		t.Fatalf("run review and host result disagree: %s vs %s", got, uncovered)
	}
	c := readCapturedReview(t, home)
	if _, ok := reviewFileLines(c, path); ok || strings.Contains(c.Input.Diff, "VALUES (5000,") || strings.Join(c.Input.Paths, ",") != covered {
		t.Fatalf("uncovered content reached the reviewer or coverage still requires it: paths=%v", c.Input.Paths)
	}
	if lines, ok := reviewFileLines(c, "sample.txt"); !ok || lines != "1: review me\n" {
		t.Fatalf("other files must still be reviewed: %v", c.Input.Files)
	}
	if len(c.Input.Uncovered) != 1 || c.Input.Uncovered[0].Path != path || !strings.HasPrefix(c.Input.Uncovered[0].Reason, reason) {
		t.Fatalf("reviewer input must name the uncovered path: %+v", c.Input.Uncovered)
	}
}

func hostReviewLargeFile(t *testing.T, bin, repo, home, target string) map[string]any {
	t.Helper()
	installFake(t, home, cleanFakeReview)
	v := axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"fake-host","id":"task-7"}}`)
	v = hostPrepare(t, bin, repo, home, v, target, "-large")
	v = hostCall(t, bin, repo, home, v, "review", "review-large", map[string]any{"grant": "round-large"})
	if v["outcome"] != "ok" {
		t.Fatalf("oversized file refused the whole review: %v", v["error"])
	}
	return hostSettle(t, bin, repo, home)
}

func TestReviewUndeclaredLargeFileIsPartial(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	writeFixtureFile(t, repo, "migrations/baseline.sql", largeMigration())
	writeFixtureFile(t, repo, "sample.txt", "review me\n")
	commitReviewFixture(t, repo, "squash migrations")
	assertPartialReview(t, hostReviewLargeFile(t, bin, repo, home, target), home, "migrations/baseline.sql", "sample.txt")
}

func TestReviewCandidateCannotDeclareItsOwnFileGenerated(t *testing.T) {
	for _, declaration := range []string{".gitattributes", ".git/info/attributes"} {
		t.Run(declaration, func(t *testing.T) {
			bin, repo, home := fixture(t)
			target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
			writeFixtureFile(t, repo, declaration, "migrations/*.sql linguist-generated\n")
			writeFixtureFile(t, repo, "migrations/baseline.sql", largeMigration())
			writeFixtureFile(t, repo, "sample.txt", "review me\n")
			commitReviewFixture(t, repo, "declare own migrations generated")
			covered := "sample.txt"
			if declaration == ".gitattributes" {
				covered = ".gitattributes,sample.txt"
			}
			v := hostReviewLargeFile(t, bin, repo, home, target)
			assertPartialReview(t, v, home, "migrations/baseline.sql", covered)
			if generated := v["host_result"].(map[string]any)["generated"]; generated != nil {
				t.Fatalf("candidate-declared path was exempted: %v", generated)
			}
		})
	}
}

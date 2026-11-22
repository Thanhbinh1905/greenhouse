package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func declareContextWindow(t *testing.T, home, window string) {
	t.Helper()
	path := filepath.Join(filepath.Dir(home), "fake-pi")
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	declared := strings.Replace(string(script), "fixture model 128K", "fixture model "+window, 1)
	if declared == string(script) {
		t.Fatal("fake pi catalog does not declare the fixture model")
	}
	if err := os.WriteFile(path, []byte(declared), 0700); err != nil {
		t.Fatal(err)
	}
}

func hostReviewWithContext(t *testing.T, bin, repo, home, target, mode, window string) map[string]any {
	t.Helper()
	installFake(t, home, mode)
	declareContextWindow(t, home, window)
	v := axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"fake-host","id":"task-7"}}`)
	v = hostPrepare(t, bin, repo, home, v, target, "-context")
	v = hostCall(t, bin, repo, home, v, "review", "review-context", map[string]any{"grant": "round-context"})
	if v["outcome"] != "ok" {
		t.Fatalf("review refused: %v", v)
	}
	return hostSettle(t, bin, repo, home)
}

func proseOf(size int) string {
	var b strings.Builder
	for i := 0; b.Len() < size; i++ {
		fmt.Fprintf(&b, "Line %d of the long design note explains one more detail.\n", i)
	}
	return b.String()
}

func TestReviewPromptThatFitsTheContextWindow(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	writeFixtureFile(t, repo, "notes.txt", proseOf(20*1024))
	writeFixtureFile(t, repo, "sample.txt", "review me\n")
	commitReviewFixture(t, repo, "add notes")

	result := hostReviewWithContext(t, bin, repo, home, target, cleanFakeReview, "40K")["host_result"].(map[string]any)
	if result["status"] != "clean-review" || result["verdict"] != "clean" || result["uncovered"] != nil {
		t.Fatalf("a prompt within the context window must be reviewed in full: %v", result)
	}
	c := readCapturedReview(t, home)
	if strings.Join(c.Input.Paths, ",") != "notes.txt,sample.txt" {
		t.Fatalf("every path must be reviewed: %v", c.Input.Paths)
	}
}

var contextReason = regexp.MustCompile(`^prompt exceeds model context \((\d+) tokens > (\d+)\)$`)

func TestReviewPromptOverTheContextWindowIsPartial(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	writeFixtureFile(t, repo, "notes.txt", proseOf(40*1024))
	writeFixtureFile(t, repo, "sample.txt", "review me\n")
	commitReviewFixture(t, repo, "add long notes")

	v := hostReviewWithContext(t, bin, repo, home, target, cleanFakeReview, "40K")
	result := v["host_result"].(map[string]any)
	if result["status"] != "partial-review" || result["verdict"] != "partial" {
		t.Fatalf("a prompt over the context window must give a partial review: %v", result)
	}
	uncovered, _ := result["uncovered"].([]any)
	if len(uncovered) != 1 {
		t.Fatalf("exactly the oversized file must be uncovered: %v", result["uncovered"])
	}
	entry := uncovered[0].(map[string]any)
	match := contextReason.FindStringSubmatch(fmt.Sprint(entry["reason"]))
	if entry["path"] != "notes.txt" || match == nil {
		t.Fatalf("uncovered reason must name the measured and allowed sizes: %v", entry)
	}
	measured, _ := strconv.Atoi(match[1])
	allowed, _ := strconv.Atoi(match[2])
	if allowed != 20000 || measured <= allowed {
		t.Fatalf("a 40K window allows half for the prompt: measured=%d allowed=%d", measured, allowed)
	}
	c := readCapturedReview(t, home)
	if _, ok := reviewFileLines(c, "notes.txt"); ok || strings.Contains(c.Input.Diff, "Line 500 of") || strings.Join(c.Input.Paths, ",") != "sample.txt" {
		t.Fatalf("uncovered content reached the reviewer: paths=%v", c.Input.Paths)
	}
	if lines, ok := reviewFileLines(c, "sample.txt"); !ok || lines != "1: review me\n" {
		t.Fatalf("the file that fits must still be reviewed: %v", c.Input.Files)
	}
	if estimate := (len(c.Raw) + 2) / 3; estimate > allowed {
		t.Fatalf("the model was called with a prompt over the allowed size: %d tokens > %d", estimate, allowed)
	}
}

func TestReviewPromptThatCannotFitIsRefused(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	writeFixtureFile(t, repo, "sample.txt", "review me\n")
	commitReviewFixture(t, repo, "small change")
	installFake(t, home, cleanFakeReview)
	declareContextWindow(t, home, "2K")
	v := axi(t, bin, repo, home, `{"version":2,"op":"attach","host":{"name":"fake-host","id":"task-7"}}`)
	v = hostPrepare(t, bin, repo, home, v, target, "-context")
	v = hostCall(t, bin, repo, home, v, "review", "review-context", map[string]any{"grant": "round-context"})
	if v["outcome"] != "refused" || v["error_code"] != "prompt_exceeds_context" || v["retryable"] != false {
		t.Fatalf("a prompt that cannot fit even without files must be refused with a typed code: %v", v)
	}
	if !strings.Contains(fmt.Sprint(v["error"]), "tokens > 1000") {
		t.Fatalf("refusal must name the measured and allowed sizes: %v", v["error"])
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(home), "invocations")); !os.IsNotExist(err) {
		t.Fatalf("the model was called for a prompt that cannot fit: %v", err)
	}
}

const contextOverflowFake = `print(json.dumps({'type':'compaction_start','reason':'overflow'}))
sys.exit(255)`

func TestReviewBackendContextOverflowIsTyped(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	writeFixtureFile(t, repo, "sample.txt", "review me\n")
	commitReviewFixture(t, repo, "small change")

	result := hostReviewWithContext(t, bin, repo, home, target, contextOverflowFake, "200K")["host_result"].(map[string]any)
	if result["status"] != "inconclusive" || result["reason"] != "context_overflow" {
		t.Fatalf("a backend context overflow must be typed, not a crash: %v", result)
	}
	if !strings.Contains(fmt.Sprint(result["diagnostic"]), "context") {
		t.Fatalf("the diagnostic must explain the overflow: %v", result["diagnostic"])
	}
}

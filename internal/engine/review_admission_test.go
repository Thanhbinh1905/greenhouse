package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func commitReviewFixture(t *testing.T, repo, message string) string {
	t.Helper()
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", message)
	return strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
}

func prepareReviewAdmission(t *testing.T, bin, repo, home, target string) map[string]any {
	t.Helper()
	installFake(t, home, "")
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": "none", "action_id": "prepare", "intent": "Review committed change", "target": target}
	b, _ := json.Marshal(q)
	v = axi(t, bin, repo, home, string(b))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	return mutate(t, bin, repo, home, v, "resolve-rules", "rules", nil)
}

func TestGitlinkReview(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"only", "mixed", "missing", "uninitialized", "truncated", "binary"} {
		t.Run(mode, func(t *testing.T) {
			bin, repo, home := fixture(t)
			sub := filepath.Join(filepath.Dir(home), "local-sub")
			if err := os.Mkdir(sub, 0700); err != nil {
				t.Fatal(err)
			}
			git(t, sub, "init", "-b", "main")
			if err := os.WriteFile(filepath.Join(sub, "code.txt"), []byte("old\n"), 0600); err != nil {
				t.Fatal(err)
			}
			old := commitReviewFixture(t, sub, "old")
			git(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", "--name", "library", sub, "deps/lib")
			target := commitReviewFixture(t, repo, "base pin")
			if err := os.WriteFile(filepath.Join(sub, "code.txt"), []byte("new\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if mode == "truncated" {
				if err := os.WriteFile(filepath.Join(sub, "code.txt"), []byte(strings.Repeat("new implementation line\n", 12000)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "binary" {
				if err := os.WriteFile(filepath.Join(sub, "code.txt"), []byte{0, 1, 2, 3}, 0600); err != nil {
					t.Fatal(err)
				}
			}
			pin := commitReviewFixture(t, sub, "new implementation")
			work := filepath.Join(repo, "deps/lib")
			git(t, work, "fetch", sub, "main")
			git(t, work, "checkout", pin)
			if mode == "mixed" {
				if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("text\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "missing" {
				pin = strings.Repeat("a", 40)
				git(t, repo, "update-index", "--cacheinfo", "160000,"+pin+",deps/lib")
			}
			if mode == "missing" {
				git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "bump")
			} else {
				commitReviewFixture(t, repo, "bump")
			}
			if mode == "uninitialized" {
				git(t, repo, "submodule", "deinit", "-f", "--", "deps/lib")
				if err := os.WriteFile(filepath.Join(repo, ".gitmodules"), []byte("invalid working tree data\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			v := prepareReviewAdmission(t, bin, repo, home, target)
			script := filepath.Join(filepath.Dir(home), "fake-pi")
			b, _ := os.ReadFile(script)
			if err := os.WriteFile(script, []byte(strings.ReplaceAll(string(b), "'file':'sample.txt'", "'file':'deps/lib'")), 0700); err != nil {
				t.Fatal(err)
			}
			v = mutate(t, bin, repo, home, v, "review", "review", nil)
			if mode == "binary" {
				if v["error_code"] != "unsupported_binary_diff" || v["retryable"] != false || !strings.Contains(v["error"].(string), "deps/lib") {
					t.Fatal(v)
				}
				return
			}
			if mode == "missing" {
				if v["error_code"] != "submodule_unavailable" || v["retryable"] != false || !strings.Contains(v["error"].(string), old) || !strings.Contains(v["error"].(string), pin) {
					t.Fatal(v)
				}
				return
			}
			if v["outcome"] != "ok" {
				t.Fatalf("gitlink admission: %v", v)
			}
			v = waitReview(t, bin, repo, home)
			review := runOf(t, v)["review"].(map[string]any)
			if review["status"] != "outstanding-findings" {
				t.Fatal(review)
			}
			var captured struct {
				Input struct {
					Files      []reviewFile
					Diff       string
					Submodules []reviewSubmoduleMetadata
				}
				Raw string `json:"raw"`
			}
			b, _ = os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
			if err := json.Unmarshal(b, &captured); err != nil {
				t.Fatal(err)
			}
			c := captured.Input
			if len(c.Submodules) != 1 || c.Submodules[0].Old != old || c.Submodules[0].New != pin || !strings.Contains(captured.Raw, "new implementation") || !strings.Contains(captured.Raw, "+new") || !strings.Contains(c.Diff, "Subproject commit "+pin) {
				t.Fatalf("missing range: %+v", c)
			}
			if mode == "truncated" && (!c.Submodules[0].Truncated || !strings.Contains(captured.Raw, "[submodule range truncated]") || len(captured.Raw) > 2*1024*1024) {
				t.Fatal("submodule range was not bounded and marked")
			}
			wantFiles := 1
			if mode == "mixed" {
				wantFiles = 2
			}
			if len(c.Files) != wantFiles || c.Files[0].Path != "deps/lib" || c.Files[0].Lines != "1: Subproject commit "+pin+"\n" {
				t.Fatalf("files: %+v", c.Files)
			}
			if mode == "mixed" && (c.Files[1].Path != "sample.txt" || c.Files[1].Lines != "1: text\n") {
				t.Fatalf("text file omitted from mixed review: %+v", c.Files)
			}
			var x Execution
			b, _ = json.Marshal(review)
			if err := json.Unmarshal(b, &x); err != nil {
				t.Fatal(err)
			}
			first := x.Result.Findings[0].ID
			result, err := verdictFrom(agentAnswer{Text: mustJSON(t, map[string]any{"head": x.Result.Head, "verdict": x.Result.Verdict, "coverage": x.Result.Coverage, "findings": x.Result.Findings}), Provider: "fixture", Model: "model"}, &x)
			if err != nil || result.Findings[0].ID != first {
				t.Fatalf("unstable identity: %v %+v", err, result)
			}
			x.Result.Findings[0].Start = 2
			x.Result.Findings[0].End = 2
			filtered, err := verdictFrom(agentAnswer{Text: mustJSON(t, map[string]any{"head": x.Result.Head, "verdict": x.Result.Verdict, "coverage": x.Result.Coverage, "findings": x.Result.Findings}), Provider: "fixture", Model: "model"}, &x)
			if err != nil || len(filtered.Findings) != 0 || len(filtered.Diagnostics) != 1 || filtered.Verdict != "clean" {
				t.Fatalf("gitlink finding outside line 1 was not filtered: %v %+v", err, filtered)
			}
		})
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}

func TestVerdictFromKeepsFindingsOverlappingChangedHunks(t *testing.T) {
	diff := "diff --git a/sample.txt b/sample.txt\n--- a/sample.txt\n+++ b/sample.txt\n@@ -2,3 +2,3 @@\n context\n-old\n+new\n context\n@@ -10,3 +10,3 @@\n context\n-old\n+new\n context\n"
	x := &Execution{Rules: InputIdentity{Candidate: "candidate"}, Paths: []string{"sample.txt"}, Diff: diff, Config: ReviewerConfig{Backend: "pi", Model: "fixture/model"}}
	makeAnswer := func(start, end int) agentAnswer {
		finding := Finding{Priority: "P1", Origin: "correctness", File: "sample.txt", Start: start, End: end, Explanation: "The changed code is incorrect", Action: "Correct it", Head: "candidate"}
		return agentAnswer{Text: mustJSON(t, map[string]any{"head": "candidate", "verdict": "findings", "coverage": []string{"sample.txt"}, "findings": []Finding{finding}}), Provider: "fixture", Model: "model"}
	}
	for _, tc := range []struct {
		name                           string
		start, end, wantStart, wantEnd int
	}{
		{"extends past hunk", 2, 8, 2, 4},
		{"spans two hunks", 2, 12, 2, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := verdictFrom(makeAnswer(tc.start, tc.end), x)
			if err != nil || got.Verdict != "findings" || len(got.Findings) != 1 || got.Findings[0].Start != tc.wantStart || got.Findings[0].End != tc.wantEnd {
				t.Fatalf("finding was not retained and clamped: err=%v verdict=%+v", err, got)
			}
		})
	}
	got, err := verdictFrom(makeAnswer(20, 21), x)
	if err != nil || got.Verdict != "clean" || len(got.Findings) != 0 || len(got.Diagnostics) != 1 || len(got.RejectedFindings) != 1 || got.RejectedFindings[0].Reason != "finding range outside changed hunks" {
		t.Fatalf("non-overlapping finding was not rejected visibly: err=%v verdict=%+v", err, got)
	}
}

func TestReviewAdmissionOverDiffLimitLeavesLargePathUncovered(t *testing.T) {
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	large := filepath.Join(repo, "docs", "evidence", "large.txt")
	if err := os.MkdirAll(filepath.Dir(large), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(large, []byte(strings.Repeat("xxx\n", 300000)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review this change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commitReviewFixture(t, repo, "large and small changes")
	v := prepareReviewAdmission(t, bin, repo, home, target)
	scriptPath := filepath.Join(filepath.Dir(home), "fake-pi")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	scriptText := strings.Replace(string(script), "'verdict':'findings'", "'verdict':'clean'", 1)
	scriptText = strings.Replace(scriptText, "'findings':[{'id':'model-id','priority':'P2','origin':'policy','file':'sample.txt','start':1,'end':1,'explanation':'Changed prose is redundant','action':'Remove repetition','rule':'unslop/plain-prose','head':b['Head']}]", "'findings':[]", 1)
	if err := os.WriteFile(scriptPath, []byte(scriptText), 0700); err != nil {
		t.Fatal(err)
	}
	v = mutate(t, bin, repo, home, v, "review", "over-diff-limit", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("large diff admission: code=%v error=%v", v["error_code"], v["error"])
	}
	v = waitReview(t, bin, repo, home)
	review := runOf(t, v)["review"].(map[string]any)
	result := review["result"].(map[string]any)
	if review["status"] != "partial-review" || result["verdict"] != "partial" {
		t.Fatalf("large path should settle partial: status=%v verdict=%v", review["status"], result["verdict"])
	}
	uncovered := review["uncovered"].([]any)
	if len(uncovered) != 1 || uncovered[0].(map[string]any)["path"] != "docs/evidence/large.txt" || uncovered[0].(map[string]any)["reason"] == "" {
		t.Fatalf("large path needs an uncovered reason: %v", uncovered)
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Input struct {
			Paths []string
			Diff  string
		}
	}
	if err := json.Unmarshal(b, &captured); err != nil {
		t.Fatal(err)
	}
	if strings.Join(captured.Input.Paths, ",") != "sample.txt" || !strings.Contains(captured.Input.Diff, "[uncovered: hunks omitted; see Uncovered]") {
		t.Fatalf("review did not retain small path and omit the large path: %+v", captured.Input)
	}
}

func TestGeneratedEvidenceOverDiffLimitUsesShortForm(t *testing.T) {
	bin, repo, home := fixture(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs", "evidence"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("docs/evidence/** linguist-generated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ordinaryPaths := []string{"z-ordinary.txt", "z-secondary.txt"}
	for _, path := range ordinaryPaths {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(strings.Repeat("old line\n", 6000)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	target := commitReviewFixture(t, repo, "mark evidence generated")
	if err := os.WriteFile(filepath.Join(repo, "docs", "evidence", "large.txt"), []byte(strings.Repeat("evidence line\n", 69000)), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range ordinaryPaths {
		if err := os.WriteFile(filepath.Join(repo, path), []byte("new line\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review this change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commitReviewFixture(t, repo, "add generated evidence")
	v := prepareReviewAdmission(t, bin, repo, home, target)
	v = mutate(t, bin, repo, home, v, "review", "generated-over-diff-limit", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("generated diff admission: code=%v error=%v", v["error_code"], v["error"])
	}
	_ = waitReview(t, bin, repo, home)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Input struct {
			Generated []GeneratedPath
			Paths     []string
			Uncovered []UncoveredPath
			Diff      string
		}
	}
	if err := json.Unmarshal(b, &captured); err != nil {
		t.Fatal(err)
	}
	if len(captured.Input.Generated) != 1 || captured.Input.Generated[0].Path != "docs/evidence/large.txt" || captured.Input.Generated[0].Added != 69000 || len(captured.Input.Diff) > 1024*1024 || strings.Contains(captured.Input.Diff, "evidence line") {
		t.Fatalf("generated evidence was not kept in short form: paths=%+v diff bytes=%d", captured.Input.Generated, len(captured.Input.Diff))
	}
	for _, path := range ordinaryPaths {
		if !slices.Contains(captured.Input.Paths, path) || slices.ContainsFunc(captured.Input.Uncovered, func(item UncoveredPath) bool { return item.Path == path }) {
			t.Fatalf("generated path consumed %s's diff budget: paths=%v uncovered=%+v", path, captured.Input.Paths, captured.Input.Uncovered)
		}
	}
}

func TestBoundedReviewDiffDoesNotSpendOrdinaryBudgetOnGeneratedSections(t *testing.T) {
	_, repo, _ := fixture(t)
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	generatedPath := "a-generated.txt"
	ordinaryPaths := []string{"z-ordinary.txt", "z-secondary.txt"}
	if err := os.WriteFile(filepath.Join(repo, generatedPath), []byte(strings.Repeat("evidence line\n", 67000)), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range ordinaryPaths {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(strings.Repeat("small line\n", 8000)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	candidate := commitReviewFixture(t, repo, "add generated and ordinary changes")
	fullDiff := git(t, repo, reviewDiffArgs(base, candidate)...)
	if len(fullDiff) <= 1024*1024 {
		t.Fatalf("fixture diff is only %d bytes", len(fullDiff))
	}
	names := append([]string{generatedPath}, ordinaryPaths...)
	diff, uncovered, err := boundedReviewDiff(repo, base, candidate, names, []GeneratedPath{{Path: generatedPath}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range ordinaryPaths {
		if slices.ContainsFunc(uncovered, func(item UncoveredPath) bool { return item.Path == path }) {
			t.Fatalf("ordinary path %s was excluded despite the generated marker replacing its section: %+v", path, uncovered)
		}
		if !strings.Contains(diff, "diff --git a/"+path+" b/"+path) {
			t.Fatalf("bounded diff omitted ordinary path %s", path)
		}
	}
	if len(diff) > 1024*1024 || !strings.Contains(diff, "diff --git a/"+generatedPath+" b/"+generatedPath+"\n[generated: hunks omitted; see Generated]") {
		t.Fatalf("bounded diff did not retain generated short form: bytes=%d", len(diff))
	}
}

func TestBoundedReviewDiffChargesOrdinarySectionsByRenderedSize(t *testing.T) {
	_, repo, _ := fixture(t)
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	prefixPath, ordinaryPath := "a-prefix.txt", "z-ordinary.txt"
	for path, content := range map[string]string{prefixPath: "p\n", ordinaryPath: "o\n"} {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	initial := commitReviewFixture(t, repo, "add prefix and ordinary paths")
	prefixDiff := git(t, repo, append(reviewDiffArgs(base, initial), "--", prefixPath)...)
	ordinaryDiff := git(t, repo, append(reviewDiffArgs(base, initial), "--", ordinaryPath)...)
	wantPrefixBytes := 1024*1024 - len(ordinaryDiff)
	contentBytes := 2 + wantPrefixBytes - len(prefixDiff)
	if contentBytes < 2 {
		t.Fatalf("cannot size prefix fixture: content bytes=%d", contentBytes)
	}
	if err := os.WriteFile(filepath.Join(repo, prefixPath), []byte(strings.Repeat("p", contentBytes-1)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	candidate := commitReviewFixture(t, repo, "fill budget before ordinary path")
	prefixDiff = git(t, repo, append(reviewDiffArgs(base, candidate), "--", prefixPath)...)
	if len(prefixDiff) != wantPrefixBytes {
		t.Fatalf("prefix diff is %d bytes, want %d", len(prefixDiff), wantPrefixBytes)
	}
	diff, uncovered, err := boundedReviewDiff(repo, base, candidate, []string{prefixPath, ordinaryPath}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) != 1024*1024 || len(uncovered) != 0 {
		t.Fatalf("ordinary section should use the remaining exact budget: diff=%d uncovered=%+v", len(diff), uncovered)
	}
}

func TestBoundedReviewDiffUsesRenderedGitlinkSectionSize(t *testing.T) {
	_, repo, home := fixture(t)
	sub := filepath.Join(filepath.Dir(home), "budget-submodule")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, sub, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(sub, "code.txt"), []byte("old submodule code\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commitReviewFixture(t, sub, "add old submodule content")
	git(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", "--name", "library", sub, "deps/lib")
	base := commitReviewFixture(t, repo, "record old gitlink")
	if err := os.WriteFile(filepath.Join(sub, "code.txt"), []byte("new submodule code\n"), 0600); err != nil {
		t.Fatal(err)
	}
	newPin := commitReviewFixture(t, sub, "update submodule content")
	work := filepath.Join(repo, "deps/lib")
	git(t, work, "fetch", sub, "main")
	git(t, work, "checkout", newPin)
	prefixPath, submodulePath := "a-prefix.txt", "deps/lib"
	if err := os.WriteFile(filepath.Join(repo, prefixPath), []byte("x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	initial := commitReviewFixture(t, repo, "add prefix and bump gitlink")
	names := []string{prefixPath, submodulePath}
	sections := make(map[string]int, len(names))
	for _, path := range names {
		args := append(reviewDiffArgs(base, initial), path)
		sections[path] = len(git(t, repo, args...))
	}
	wantPrefix := 1024*1024 - sections[submodulePath]
	delta := wantPrefix - sections[prefixPath]
	if delta < 0 {
		t.Fatalf("prefix fixture already exceeds limit by %d bytes", -delta)
	}
	if err := os.WriteFile(filepath.Join(repo, prefixPath), []byte(strings.Repeat("x", delta+1)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	candidate := commitReviewFixture(t, repo, "fill remaining budget")
	diff, uncovered, err := boundedReviewDiff(repo, base, candidate, names, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) != 1024*1024 || len(uncovered) != 0 {
		t.Fatalf("gitlink section should use its rendered size: diff=%d uncovered=%+v", len(diff), uncovered)
	}
}

func TestBoundedReviewDiffMarksRemainingPathsAtExactLimit(t *testing.T) {
	_, repo, _ := fixture(t)
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	first := "a-0000000000.txt"
	marker := fmt.Sprintf("diff --git a/%s b/%s\n[uncovered: hunks omitted; see Uncovered]\n", "b-0000000000.txt", "b-0000000000.txt")
	if err := os.WriteFile(filepath.Join(repo, first), []byte("x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	initialHead := commitReviewFixture(t, repo, "add prefix file")
	initialDiff := git(t, repo, reviewDiffArgs(base, initialHead)...)
	wantPrefixBytes := 1024*1024 - len(marker)
	contentBytes := 2 + wantPrefixBytes - len(initialDiff)
	if err := os.WriteFile(filepath.Join(repo, first), []byte(strings.Repeat("x", contentBytes-1)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	prefixHead := commitReviewFixture(t, repo, "fill diff to marker boundary")
	prefixDiff := git(t, repo, reviewDiffArgs(base, prefixHead)...)
	if len(prefixDiff) != wantPrefixBytes {
		t.Fatalf("prefix diff is %d bytes, want %d", len(prefixDiff), wantPrefixBytes)
	}
	for name, content := range map[string]string{"b-0000000000.txt": strings.Repeat("b", 1024) + "\n", "c-0000000000.txt": "c\n"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	candidate := commitReviewFixture(t, repo, "add paths after marker boundary")
	diff, uncovered, err := boundedReviewDiff(repo, base, candidate, []string{first, "b-0000000000.txt", "c-0000000000.txt"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) != 1024*1024 {
		t.Fatalf("bounded diff is %d bytes, want exactly 1 MiB", len(diff))
	}
	if len(uncovered) != 2 || uncovered[0].Path != "b-0000000000.txt" || uncovered[1].Path != "c-0000000000.txt" || uncovered[0].Reason != "diff_bound" || uncovered[1].Reason != "diff_bound" {
		t.Fatalf("paths after the marker boundary must be uncovered with diff_bound: %+v", uncovered)
	}
}

func TestBoundedReviewDiffDoesNotExceedLimitWhenMarkingPaths(t *testing.T) {
	_, repo, _ := fixture(t)
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	var names []string
	for i := 0; i < 500; i++ {
		name := fmt.Sprintf("file-%03d.txt", i)
		names = append(names, name)
		if err := os.WriteFile(filepath.Join(repo, name), []byte(strings.Repeat("x", 8192)+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	candidate := commitReviewFixture(t, repo, "add many bounded diff files")
	diff, uncovered, err := boundedReviewDiff(repo, base, candidate, names, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) > 1024*1024 {
		t.Fatalf("bounded diff is %d bytes, over the 1 MiB limit", len(diff))
	}
	if len(uncovered) == 0 {
		t.Fatal("paths omitted at the diff limit must be listed as uncovered")
	}
	for _, path := range uncovered {
		if path.Reason != "diff_bound" {
			t.Fatalf("path %s has reason %q, want diff_bound", path.Path, path.Reason)
		}
	}
}

func TestExcludeUncoveredReviewInput(t *testing.T) {
	names := []string{"covered.txt", "omitted.txt", "covered-generated.txt", "submodule"}
	files := []reviewFile{{"covered.txt", "covered content"}, {"omitted.txt", "omitted content"}, {"covered-generated.txt", "generated output"}, {"generator.txt", "shared generator"}, {"exclusive-generator.txt", "exclusive generator"}}
	modules := []reviewSubmodule{{Path: "submodule", Diff: "submodule content"}}
	generated := []GeneratedPath{{Path: "omitted.txt", Generators: []string{"exclusive-generator.txt"}}, {Path: "covered-generated.txt", Generators: []string{"generator.txt"}}}
	uncovered := []UncoveredPath{{Path: "omitted.txt", Reason: "diff_bound"}, {Path: "submodule", Reason: "diff_bound"}}
	paths, filteredFiles, filteredModules, filteredGenerated := excludeUncoveredReviewInput(names, files, modules, generated, uncovered)
	if !slices.Equal(paths, []string{"covered.txt", "covered-generated.txt"}) {
		t.Fatalf("review paths: %v", paths)
	}
	for _, path := range []string{"covered.txt", "covered-generated.txt", "generator.txt"} {
		if !slices.ContainsFunc(filteredFiles, func(file reviewFile) bool { return file.Path == path }) {
			t.Fatalf("review file %s was removed: %+v", path, filteredFiles)
		}
	}
	if slices.ContainsFunc(filteredFiles, func(file reviewFile) bool {
		return file.Path == "omitted.txt" || file.Path == "exclusive-generator.txt"
	}) {
		t.Fatalf("uncovered content remained in reviewer Files: %+v", filteredFiles)
	}
	if len(filteredModules) != 0 || len(filteredGenerated) != 1 || filteredGenerated[0].Path != "covered-generated.txt" {
		t.Fatalf("uncovered metadata remained: submodules=%+v generated=%+v", filteredModules, filteredGenerated)
	}
	input, err := renderReviewInput(struct {
		Paths      []string
		Files      []reviewFile
		Submodules []reviewSubmodule
		Generated  []GeneratedPath
		Uncovered  []UncoveredPath
	}{paths, filteredFiles, filteredModules, filteredGenerated, uncovered}, "", filteredFiles, filteredModules)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"omitted content", "exclusive generator", "submodule content"} {
		if strings.Contains(string(input), content) {
			t.Fatalf("uncovered content %q reached reviewer input", content)
		}
	}
}

func TestReviewAdmissionOmitsUncoveredFilesFromReviewerInput(t *testing.T) {
	bin, repo, home := fixture(t)
	var names []string
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("large-%02d.txt", i)
		names = append(names, name)
		if err := os.WriteFile(filepath.Join(repo, name), []byte(strings.Repeat("old line\n", 8000)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	target := commitReviewFixture(t, repo, "add large base files")
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("new line\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review this change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commitReviewFixture(t, repo, "shrink files and add sample")
	diff := git(t, repo, reviewDiffArgs(target, "HEAD")...)
	if len(diff) <= 1024*1024 {
		t.Fatalf("fixture diff is only %d bytes", len(diff))
	}
	v := prepareReviewAdmission(t, bin, repo, home, target)
	v = mutate(t, bin, repo, home, v, "review", "omit-uncovered-files", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("large diff admission: code=%v error=%v", v["error_code"], v["error"])
	}
	_ = waitReview(t, bin, repo, home)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Input struct {
			Paths        []string
			Files        []reviewFile
			Generated    []GeneratedPath
			Submodules   []reviewSubmodule
			ChangedHunks map[string][][2]int
			Uncovered    []UncoveredPath
		}
	}
	if err := json.Unmarshal(b, &captured); err != nil {
		t.Fatal(err)
	}
	uncovered := map[string]bool{}
	for _, path := range captured.Input.Uncovered {
		uncovered[path.Path] = true
	}
	if len(uncovered) == 0 {
		t.Fatal("large diff did not leave any paths uncovered")
	}
	hasDiffBound := false
	for _, path := range captured.Input.Uncovered {
		hasDiffBound = hasDiffBound || path.Reason == "diff_bound"
	}
	if !hasDiffBound {
		t.Fatalf("diff-bound paths were not listed: %+v", captured.Input.Uncovered)
	}
	for _, path := range captured.Input.Paths {
		if uncovered[path] {
			t.Fatalf("uncovered path %s was included in reviewer Paths", path)
		}
	}
	for _, file := range captured.Input.Files {
		if uncovered[file.Path] {
			t.Fatalf("uncovered path %s was included in reviewer Files", file.Path)
		}
	}
	for _, path := range captured.Input.Generated {
		if uncovered[path.Path] {
			t.Fatalf("uncovered path %s was included in reviewer Generated", path.Path)
		}
	}
	for _, module := range captured.Input.Submodules {
		if uncovered[module.Path] {
			t.Fatalf("uncovered path %s was included in reviewer Submodules", module.Path)
		}
	}
	for path := range captured.Input.ChangedHunks {
		if uncovered[path] {
			t.Fatalf("uncovered path %s was included in reviewer ChangedHunks", path)
		}
	}
}

func TestReviewInputLargeFile(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	content := strings.Repeat("unchanged candidate line\n", 60000)
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	target := commitReviewFixture(t, repo, "large base")
	content = strings.Replace(content, "unchanged candidate line\n", "changed candidate line\n", 1)
	if err := os.WriteFile(filepath.Join(repo, "sample.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	commitReviewFixture(t, repo, "small hunk")
	v := prepareReviewAdmission(t, bin, repo, home, target)
	v = mutate(t, bin, repo, home, v, "review", "large", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("large file admission: %v", v["error"])
	}
	v = waitReview(t, bin, repo, home)
	if runOf(t, v)["status"] != "outstanding-findings" {
		t.Fatal(runOf(t, v)["review"])
	}
	b, _ := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	var c struct {
		Input struct {
			Files []reviewFile
			Diff  string
		}
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if len(b) > 2*1024*1024 || len(c.Input.Files) != 1 || len(c.Input.Files[0].Lines) > 64*1024 || !strings.HasPrefix(c.Input.Files[0].Lines, "1: changed candidate line\n") || !strings.Contains(c.Input.Files[0].Lines, "...") {
		t.Fatalf("unbounded file bundle: %d", len(b))
	}
	complete := git(t, repo, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", "--no-color", "--no-relative", "--src-prefix=a/", "--dst-prefix=b/", "--binary", target, "HEAD", "--")
	if complete != c.Input.Diff {
		t.Fatal("diff lost content")
	}
}

func TestReviewInputTooLarge(t *testing.T) {
	files := []reviewFile{{"large.go", strings.Repeat("line\n", 100)}}
	err := checkReviewInput(make([]byte, 2*1024*1024+1), "complete diff", files, []byte(`{}`), nil, nil)
	if err == nil {
		t.Fatal("over-budget input admitted")
	}
	v := refusalResponse(Response{Version: 2}, err)
	b, _ := json.Marshal(v)
	var response map[string]any
	if err := json.Unmarshal(b, &response); err != nil {
		t.Fatal(err)
	}
	if response["outcome"] != "refused" || response["error_code"] != "review_input_too_large" || response["retryable"] != false {
		t.Fatal(response)
	}
	for _, field := range []string{"diff=13", "files=517", "criteria=2", "responses=0", "total=2097153", "large.go="} {
		if !strings.Contains(v.Error, field) {
			t.Fatalf("missing size contributor %s: %s", field, v.Error)
		}
	}
}

func TestReviewInputWindowsAndAggregate(t *testing.T) {
	t.Parallel()
	_, repo, _ := fixture(t)
	var names []string
	for i := 0; i < 20; i++ {
		path := fmt.Sprintf("file-%02d.txt", i)
		names = append(names, path)
		if err := os.WriteFile(filepath.Join(repo, path), []byte(strings.Repeat("unchanged line\n", 3000)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	target := commitReviewFixture(t, repo, "base files")
	for _, path := range names {
		content := strings.Repeat("unchanged line\n", 3000)
		lines := strings.Split(content, "\n")
		lines[2499] = "changed middle"
		lines[2999] = "changed end"
		if err := os.WriteFile(filepath.Join(repo, path), []byte(strings.Join(lines, "\n")), 0600); err != nil {
			t.Fatal(err)
		}
	}
	head := commitReviewFixture(t, repo, "change two hunks")
	diff := git(t, repo, "diff", target, head, "--")
	files, _, err := candidateFiles(repo, head, names, diff, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(names) {
		t.Fatalf("coverage lost: %d files", len(files))
	}
	total := 0
	for _, file := range files {
		total += len(file.Lines)
		if !strings.Contains(file.Lines, "2500: changed middle\n") || !strings.Contains(file.Lines, "3000: changed end\n") || !strings.HasPrefix(file.Lines, "...\n") || strings.Contains(file.Lines, "\n1: unchanged") {
			t.Fatalf("wrong windows: %s", file.Lines)
		}
	}
	if total > 768*1024 {
		t.Fatalf("aggregate Files exceeded: %d", total)
	}
}

func TestReviewBinaryRefusal(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "image.bin"), []byte{0, 1, 2, 3}, 0600); err != nil {
		t.Fatal(err)
	}
	commitReviewFixture(t, repo, "binary")
	v := prepareReviewAdmission(t, bin, repo, home, target)
	v = mutate(t, bin, repo, home, v, "review", "binary", nil)
	if v["error_code"] != "unsupported_binary_diff" || v["retryable"] != false || !strings.Contains(v["error"].(string), "image.bin") {
		t.Fatal(v)
	}
}

func TestReviewInputUnevenFilesFitAggregate(t *testing.T) {
	_, repo, _ := fixture(t)
	var names []string
	for i := 0; i < 20; i++ {
		path := fmt.Sprintf("new-%02d.txt", i)
		names = append(names, path)
		content := "small\n"
		if i == 0 {
			content = strings.Repeat("new content\n", 3000)
		}
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	head := commitReviewFixture(t, repo, "uneven additions")
	diff := git(t, repo, "diff", target, head, "--")
	files, _, err := candidateFiles(repo, head, names, diff, nil)
	if err != nil {
		t.Fatalf("files fit aggregate budget: %v", err)
	}
	if len(files) != 20 || !strings.Contains(files[0].Lines, "3000: new content\n") || strings.Contains(files[0].Lines, "...\n") {
		t.Fatal("full content lost despite available budget")
	}
}

func TestReviewAddedAndRemovedGitlinksWithoutLocalObjects(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"removed", "added"} {
		t.Run(mode, func(t *testing.T) {
			bin, repo, home := fixture(t)
			sub := filepath.Join(filepath.Dir(home), "local-sub")
			if err := os.Mkdir(sub, 0700); err != nil {
				t.Fatal(err)
			}
			git(t, sub, "init", "-b", "main")
			if err := os.WriteFile(filepath.Join(sub, "code.txt"), []byte("implementation\n"), 0600); err != nil {
				t.Fatal(err)
			}
			pin := commitReviewFixture(t, sub, "submodule implementation")
			target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
			git(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", "--name", "library", sub, "deps/lib")
			head := commitReviewFixture(t, repo, "add submodule")
			if mode == "removed" {
				target = head
				git(t, repo, "rm", "-f", "deps/lib")
				git(t, repo, "rm", "-f", ".gitmodules")
				commitReviewFixture(t, repo, "remove submodule and mapping")
			} else {
				git(t, repo, "submodule", "deinit", "-f", "--", "deps/lib")
			}
			if err := os.RemoveAll(filepath.Join(repo, ".git", "modules", "library")); err != nil {
				t.Fatal(err)
			}
			v := prepareReviewAdmission(t, bin, repo, home, target)
			installFake(t, home, `v={"head":b["Head"],"verdict":"clean","coverage":b["Paths"],"findings":[]}
print(json.dumps({"type":"message_end","message":{"role":"assistant","provider":"fixture","model":"model","stopReason":"stop","content":[{"type":"text","text":json.dumps(v)}]}}))
print(json.dumps({"type":"agent_settled","aborted":False}))
sys.exit(0)
`)
			v = mutate(t, bin, repo, home, v, "review", "one-sided-gitlink", nil)
			if v["outcome"] != "ok" {
				t.Fatalf("%s gitlink review refused: code=%v error=%v", mode, v["error_code"], v["error"])
			}
			review := runOf(t, waitReview(t, bin, repo, home))["review"].(map[string]any)
			if review["status"] != "clean-review" {
				t.Fatalf("%s review did not complete: %v", mode, review)
			}
			data, err := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
			if err != nil {
				t.Fatal(err)
			}
			var capture struct {
				Input struct {
					Paths      []string
					Files      []reviewFile
					Submodules []reviewSubmoduleMetadata
				}
			}
			if err := json.Unmarshal(data, &capture); err != nil {
				t.Fatal(err)
			}
			want := reviewSubmodule{Path: "deps/lib", New: pin}
			if mode == "removed" {
				want.Old, want.New = pin, ""
			}
			if len(capture.Input.Submodules) != 1 || capture.Input.Submodules[0] != (reviewSubmoduleMetadata{Path: want.Path, Old: want.Old, New: want.New, Truncated: want.Truncated}) {
				t.Fatalf("%s gitlink metadata: got %+v, want %+v", mode, capture.Input.Submodules, want)
			}
			if strings.Join(capture.Input.Paths, ",") != ".gitmodules,deps/lib" {
				t.Fatalf("%s gitlink lost coverage: %v", mode, capture.Input.Paths)
			}
			if mode == "removed" && len(capture.Input.Files) != 0 {
				t.Fatalf("deleted paths retained candidate content: %+v", capture.Input.Files)
			}
			if mode == "added" && (len(capture.Input.Files) != 2 || capture.Input.Files[1] != (reviewFile{Path: "deps/lib", Lines: "1: Subproject commit " + pin + "\n"})) {
				t.Fatalf("added gitlink lost candidate content: %+v", capture.Input.Files)
			}
		})
	}
}

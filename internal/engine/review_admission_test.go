package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
					Submodules []reviewSubmodule
				}
			}
			b, _ = os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
			if err := json.Unmarshal(b, &captured); err != nil {
				t.Fatal(err)
			}
			c := captured.Input
			if len(c.Submodules) != 1 || c.Submodules[0].Old != old || c.Submodules[0].New != pin || !strings.Contains(c.Submodules[0].Log, "new implementation") || !strings.Contains(c.Submodules[0].Diff, "+new") || !strings.Contains(c.Diff, "Subproject commit "+pin) {
				t.Fatalf("missing range: %+v", c)
			}
			if mode == "truncated" && (!c.Submodules[0].Truncated || !strings.Contains(c.Submodules[0].Diff, "[submodule range truncated]") || len(c.Submodules[0].Log)+len(c.Submodules[0].Diff) > 256*1024+100) {
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

func TestReviewInputLargeFile(t *testing.T) {
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
	for _, field := range []string{"diff=", "files=", "criteria=", "responses=", "total=2097153", "large.go="} {
		if !strings.Contains(v.Error, field) {
			t.Fatalf("missing size contributor %s: %s", field, v.Error)
		}
	}
}

func TestReviewInputWindowsAndAggregate(t *testing.T) {
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
	files, err := candidateFiles(repo, head, names, diff)
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
	files, err := candidateFiles(repo, head, names, diff)
	if err != nil {
		t.Fatalf("files fit aggregate budget: %v", err)
	}
	if len(files) != 20 || !strings.Contains(files[0].Lines, "3000: new content\n") || strings.Contains(files[0].Lines, "...\n") {
		t.Fatal("full content lost despite available budget")
	}
}

func TestReviewAddedAndRemovedGitlinksWithoutLocalObjects(t *testing.T) {
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
					Submodules []reviewSubmodule
				}
			}
			if err := json.Unmarshal(data, &capture); err != nil {
				t.Fatal(err)
			}
			want := reviewSubmodule{Path: "deps/lib", New: pin}
			if mode == "removed" {
				want.Old, want.New = pin, ""
			}
			if len(capture.Input.Submodules) != 1 || capture.Input.Submodules[0] != want {
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

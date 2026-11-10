package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecutableFindingInSpacePath(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	path := filepath.Join(repo, "has space.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nold\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "has space.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "base")
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(path, []byte("one\ntwo\nchanged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "has space.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "candidate")
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	installFake(t, home, "")
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 4, "rules": []string{"unslop", "no-comments"}, "reviewer_backend": "pi", "reviewer_model": "fixture/model", "fixer_backend": "pi", "fixer_model": "fixture/model"}
	v = mutate(t, bin, repo, home, v, "apply", "fixture-model", settings)
	fake := filepath.Join(filepath.Dir(home), "fake-pi")
	script, err := os.ReadFile(fake)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(script), "'sample.txt'", "'has space.txt'")
	text = strings.Replace(text, "'start':1,'end':1", "'start':3,'end':3", 1)
	if err := os.WriteFile(fake, []byte(text), 0700); err != nil {
		t.Fatal(err)
	}
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": "none", "action_id": "prepare", "intent": "Review line three", "target": target}
	b, _ := json.Marshal(q)
	v = axi(t, bin, repo, home, string(b))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	v = mutate(t, bin, repo, home, v, "resolve-rules", "resolve", nil)
	v = mutate(t, bin, repo, home, v, "review", "review", nil)
	v = waitReview(t, bin, repo, home)
	if runOf(t, v)["status"] != "outstanding-findings" {
		review := runOf(t, v)["review"].(map[string]any)
		t.Fatalf("valid space-path finding rejected: %v", review["log"])
	}
	findings := runOf(t, v)["findings"].([]any)
	f := findings[0].(map[string]any)
	if f["file"] != "has space.txt" || f["start"] != float64(3) || runOf(t, v)["review_used"] != float64(1) {
		t.Fatal(v)
	}
}

func TestExecutableCancellationBoundsInheritedPipes(t *testing.T) {
	t.Parallel()
	testInheritedPipes(t, true)
}
func TestExecutableExitBoundsInheritedPipes(t *testing.T) {
	t.Parallel()
	testInheritedPipes(t, false)
}
func testInheritedPipes(t *testing.T, cancel bool) {
	t.Helper()
	bin, repo, home, v := prepareFixture(t)
	root := filepath.Dir(home)
	mode := `import subprocess
child=subprocess.Popen([sys.executable,'-c',"import time; time.sleep(5); open("+repr(root+'/descendant-finished')+",'w').write('done')"])
with open(root+'/descendant-ready','w') as f: f.write(str(child.pid))
`
	if cancel {
		mode += "\ntime.sleep(30)"
	}
	installFake(t, home, mode)
	t.Cleanup(func() {
		pid, _ := os.ReadFile(filepath.Join(root, "descendant-ready"))
		deadline := time.Now().Add(30 * time.Second)
		for {
			if _, err := os.Stat("/proc/" + string(pid)); len(pid) > 0 && os.IsNotExist(err) {
				break
			}
			if time.Now().After(deadline) {
				t.Error("fixture descendant did not exit")
				return
			}
			time.Sleep(time.Millisecond)
		}
		if _, err := os.Stat(filepath.Join(root, "descendant-finished")); err == nil {
			t.Error("the executor runner left a reviewer descendant running")
		}
	})
	v = mutate(t, bin, repo, home, v, "review", "start", nil)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "descendant-ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("descendant fixture did not start")
		}
		time.Sleep(time.Millisecond)
	}
	started := time.Now()
	if cancel {
		v = mutate(t, bin, repo, home, v, "cancel", "cancel", nil)
	}
	v = waitReview(t, bin, repo, home)
	elapsed := time.Since(started)
	want := "inconclusive"
	if cancel {
		want = "cancelled"
	}
	if runOf(t, v)["status"] != want || elapsed > 3*time.Second {
		t.Fatalf("executor waited for inherited pipes: status=%v elapsed=%v", runOf(t, v)["status"], elapsed)
	}
	if runOf(t, v)["review_used"] != float64(1) {
		t.Fatal("cancel erased consumption")
	}
}

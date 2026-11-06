package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "HOME="+dir)
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("git: %s %v", b, e)
	}
	return string(b)
}
func fixture(t *testing.T) (string, string, string) {
	t.Helper()
	root, err := os.MkdirTemp("", "gh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	bin := filepath.Join(root, "greenhouse")
	c := exec.Command("go", "build", "-race", "-o", bin, ".")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("build: %s %v", b, e)
	}
	repo := filepath.Join(root, "repo")
	os.Mkdir(repo, 0700)
	git(t, repo, "init", "-b", "main")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	home := filepath.Join(root, "state")
	t.Cleanup(func() {
		b, _ := os.ReadFile(filepath.Join(home, "pid"))
		var pid int
		json.Unmarshal(b, &pid)
		if pid > 0 {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	})
	return bin, repo, home
}
func axi(t *testing.T, bin, repo, home, request string, env ...string) map[string]any {
	t.Helper()
	c := exec.Command(bin, "axi")
	c.Dir = repo
	c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "HOME="+filepath.Dir(home), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	c.Env = append(c.Env, env...)
	c.Stdin = strings.NewReader(request)
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("axi: %v %s", e, b)
	}
	var r map[string]any
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatalf("json %s: %v", b, e)
	}
	return r
}
func TestConcurrentAttach(t *testing.T) {
	bin, repo, home := fixture(t)
	var a, b map[string]any
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`) }()
	go func() { defer wg.Done(); b = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`) }()
	wg.Wait()
	if a["run_id"] == nil || a["run_id"] != b["run_id"] {
		t.Fatalf("duplicate runs: %v %v", a, b)
	}
}

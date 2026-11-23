package terminal

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLineLaunchReviewsCommittedBranchWithoutInput(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	stateHome := filepath.Join(os.TempDir(), "gh-t190-"+filepath.Base(root))
	binDir := filepath.Join(root, "bin")
	for _, dir := range []string{repo, home, binDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	gitTest(t, repo, "init", "-q", "-b", "main")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	writeTestFile(t, repo, "sample.txt", "base\n")
	gitTest(t, repo, "add", "sample.txt")
	gitTest(t, repo, "commit", "-qm", "base")
	gitTest(t, repo, "checkout", "-qb", "feature")
	writeTestFile(t, repo, "sample.txt", "changed\n")
	gitTest(t, repo, "commit", "-qam", "Improve sample handling")
	writeTestFile(t, binDir, "pi", fakeReviewAgent)
	if err := os.Chmod(filepath.Join(binDir, "pi"), 0700); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(root, "greenhouse")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/greenhouse")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build greenhouse: %v\n%s", err, output)
	}
	cmd := exec.Command(binary)
	cmd.Dir = repo
	cmd.Env = testEnvironment("PATH="+binDir+":"+os.Getenv("PATH"), "HOME="+home,
		"GREENHOUSE_HOME="+stateHome, "TERM=dumb")
	term := &outputCapture{output: &bytes.Buffer{}}
	cmd.Stdout, cmd.Stderr = term, term
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		stop := exec.Command(binary, "stop")
		stop.Dir, stop.Env = repo, cmd.Env
		_ = stop.Run()
		_ = os.RemoveAll(stateHome)
	})

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(term.text(), "Stage: independent-review | outstanding-findings") {
		select {
		case err := <-done:
			t.Fatalf("greenhouse exited before findings: %v\n%s", err, term.text())
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}
	text := term.text()
	for _, want := range []string{"Stage: independent-review | outstanding-findings", "Target: main @ ", "Intent: Improve sample handling", "Intent source: commit messages + branch feature", "Outstanding P2 sample.txt:1 Sample issue found", "Rule unslop | resolved", "Rule no-comments | resolved"} {
		if !strings.Contains(text, want) {
			t.Fatalf("terminal did not show %q:\n%s", want, text)
		}
	}
	_, _ = stdin.Write([]byte("q\n"))
	_ = stdin.Close()
	if err := <-done; err != nil {
		t.Fatalf("detach: %v\n%s", err, term.text())
	}
}

func TestLineLaunchExplainsMissingCommit(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	stateHome := filepath.Join(os.TempDir(), "gh-t190-no-commit-"+filepath.Base(root))
	binDir := filepath.Join(root, "bin")
	for _, dir := range []string{repo, home, binDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "init", "-q", "-b", "feature")
	binary := filepath.Join(root, "greenhouse")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/greenhouse")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build greenhouse: %v\n%s", err, output)
	}
	cmd := exec.Command(binary)
	cmd.Dir = repo
	cmd.Env = testEnvironment("PATH="+binDir, "HOME="+home, "GREENHOUSE_HOME="+stateHome, "TERM=dumb")
	output, err := cmd.CombinedOutput()
	if err == nil || strings.TrimSpace(string(output)) != "no committed changes to review. Commit changes on this branch and run greenhouse again" {
		t.Fatalf("missing-commit guidance: error=%v output=%q", err, output)
	}
}

func TestLineLaunchRefusesAnEmptyDiffEvenWithAnOpenPullRequest(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	stateHome, err := os.MkdirTemp("", "gh-t190-empty-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateHome) })
	binDir := filepath.Join(root, "bin")
	for _, dir := range []string{repo, home, binDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "init", "-q", "-b", "main")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	writeTestFile(t, repo, "sample.txt", "base\n")
	gitTest(t, repo, "add", "sample.txt")
	gitTest(t, repo, "commit", "-qm", "base")
	gitTest(t, repo, "checkout", "-qb", "feature")
	gitTest(t, repo, "remote", "add", "origin", "git@github.com:acme/garden.git")
	writeTestFile(t, binDir, "pi", fakeReviewAgent)
	writeTestFile(t, binDir, "gh", "#!/bin/sh\nprintf '%s\\n' '{\"body\":\"Improve everything.\",\"state\":\"OPEN\"}'\n")
	for _, name := range []string{"pi", "gh"} {
		if err := os.Chmod(filepath.Join(binDir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(root, "greenhouse")
	if output, err := exec.Command("go", "build", "-o", binary, "../../cmd/greenhouse").CombinedOutput(); err != nil {
		t.Fatalf("build greenhouse: %v\n%s", err, output)
	}
	cmd := exec.Command(binary)
	cmd.Dir = repo
	cmd.Env = testEnvironment("PATH="+binDir, "HOME="+home, "GREENHOUSE_HOME="+stateHome, "TERM=dumb")
	output, err := cmd.CombinedOutput()
	if err == nil || strings.TrimSpace(string(output)) != "no committed changes to review. Commit changes on this branch and run greenhouse again" {
		t.Fatalf("empty-diff guidance: error=%v output=%q", err, output)
	}
	stop := exec.Command(binary, "stop")
	stop.Dir, stop.Env = repo, cmd.Env
	_ = stop.Run()
}

func TestLineLaunchExplainsMissingAgent(t *testing.T) {
	for name, pi := range map[string]string{
		"not installed":      "",
		"installed no login": "#!/bin/sh\necho 'provider model context'\n",
	} {
		t.Run(name, func(t *testing.T) { explainMissingAgent(t, pi) })
	}
}

func explainMissingAgent(t *testing.T, pi string) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	stateHome := filepath.Join(os.TempDir(), "gh-t190-no-agent-"+filepath.Base(root))
	binDir := filepath.Join(root, "bin")
	for _, dir := range []string{repo, home, binDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}
	if pi != "" {
		writeTestFile(t, binDir, "pi", pi)
		if err := os.Chmod(filepath.Join(binDir, "pi"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	gitTest(t, repo, "init", "-q", "-b", "main")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	writeTestFile(t, repo, "sample.txt", "committed\n")
	gitTest(t, repo, "add", "sample.txt")
	gitTest(t, repo, "commit", "-qm", "base")
	gitTest(t, repo, "checkout", "-qb", "feature")
	writeTestFile(t, repo, "sample.txt", "changed\n")
	gitTest(t, repo, "commit", "-qam", "Improve sample handling")
	binary := filepath.Join(root, "greenhouse")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/greenhouse")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build greenhouse: %v\n%s", err, output)
	}
	cmd := exec.Command(binary)
	cmd.Dir = repo
	cmd.Env = testEnvironment("PATH="+binDir, "HOME="+home, "GREENHOUSE_HOME="+stateHome, "TERM=dumb")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("greenhouse: %v\n%s", err, output)
	}
	if got, want := strings.TrimSpace(string(output)), "No signed-in coding agent found. Sign in with `pi` then /login, `claude auth login` or `codex login`, then run greenhouse again."; got != want {
		t.Fatalf("missing-agent guidance = %q, want %q", got, want)
	}
	stop := exec.Command(binary, "stop")
	stop.Dir, stop.Env = repo, testEnvironment("PATH="+binDir, "HOME="+home, "GREENHOUSE_HOME="+stateHome)
	if output, err := stop.CombinedOutput(); err != nil {
		t.Fatalf("stop daemon: %v\n%s", err, output)
	}
	_ = os.RemoveAll(stateHome)
}

const fakeReviewAgent = `#!/usr/bin/python3
import json,sys
if '--list-models' in sys.argv:
 print('provider model context')
 print('fixture default 1K 1K no no')
 sys.exit(0)
raw=sys.stdin.read()
header=raw.split('\n\nDiff:\n',1)[0]
b=json.loads(header)
v={'head':b['Head'],'verdict':'findings','coverage':b['Paths'],'findings':[{'id':'sample','priority':'P2','origin':'correctness','file':b['Paths'][0],'start':1,'end':1,'explanation':'Sample issue found','action':'Update the sample','rule':'','head':b['Head']}]}
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'default','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
`

type outputCapture struct {
	output *bytes.Buffer
	mu     sync.Mutex
}

func (c *outputCapture) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.Write(data)
}

func (c *outputCapture) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.String()
}

func testEnvironment(overrides ...string) []string {
	keys := make(map[string]bool, len(overrides))
	for _, value := range overrides {
		key, _, _ := strings.Cut(value, "=")
		keys[key] = true
	}
	var env []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !keys[key] {
			env = append(env, value)
		}
	}
	return append(env, overrides...)
}

func gitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

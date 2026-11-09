package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreshInstalledAgentReview(t *testing.T) {
	bin, repo, state := fixture(t)
	target := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "checkout", "-b", "feature")
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	git(t, repo, "add", "sample.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=t@example.invalid", "commit", "-m", "change")
	agents := filepath.Join(filepath.Dir(state), "bin")
	os.Mkdir(agents, 0700)
	script := `#!/usr/bin/python3
import json,os,sys
assert os.sched_getaffinity(0) == os.sched_getaffinity(os.getppid())
assert '--no-extensions' in sys.argv and '--no-mcp' in sys.argv
assert '--no-context-files' in sys.argv and '--no-session' in sys.argv
assert '--model' not in sys.argv
assert os.environ.get('PI_CODING_AGENT_DIR')
with open(os.path.join(os.environ['PI_CODING_AGENT_DIR'],'auth.json')) as f: assert f.read() == 'fixture native login'
if os.environ.get('EXPECT_NATIVE_READ_ONLY'):
 try:
  fd=os.open(os.path.join(os.environ['PI_CODING_AGENT_DIR'],'auth.json'),os.O_WRONLY)
 except PermissionError: pass
 else:
  os.close(fd)
  raise AssertionError('native credentials are writable')
lock=os.path.join(os.environ['PI_CODING_AGENT_DIR'],'auth.json.lock')
os.mkdir(lock);os.rmdir(lock)
assert 'PI_CODING_AGENT_SESSION_ID' not in os.environ
b=json.load(sys.stdin)
v={'head':b['Head'],'verdict':'clean','coverage':b['Paths'],'findings':[]}
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'native','model':'default','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
`
	os.WriteFile(filepath.Join(agents, "pi"), []byte(script), 0700)
	nativeHome := filepath.Join(filepath.Dir(state), "native-login")
	os.Mkdir(nativeHome, 0700)
	os.WriteFile(filepath.Join(nativeHome, "auth.json"), []byte("fixture native login"), 0600)
	env := []string{"PATH=" + agents + ":/usr/bin:/bin", "PI_CODING_AGENT_DIR=" + nativeHome, "PI_CODING_AGENT_SESSION_ID=author-session"}
	if _, err := landlockABI(); err == nil {
		env = append(env, "EXPECT_NATIVE_READ_ONLY=1")
	}
	v := axi(t, bin, repo, state, `{"version":1,"op":"attach"}`, env...)
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": v["gate_id"], "action_id": "native-prepare", "intent": "Add sample", "target": target}
	b, _ := json.Marshal(q)
	v = axi(t, bin, repo, state, string(b), env...)
	if v["outcome"] != "ok" {
		t.Fatalf("fresh installed-agent preparation refused: %v", v["error"])
	}
	v = mutate(t, bin, repo, state, v, "resolve-rules", "native-rules", nil)
	mutate(t, bin, repo, state, v, "review", "native-review", nil)
	v = waitReview(t, bin, repo, state)
	if runOf(t, v)["status"] != "clean-review" {
		t.Fatalf("native default review failed: %v", v)
	}
	for _, name := range []string{"reviewer.json", "providers.json", "backends.json", "fixer.json"} {
		if _, err := os.Stat(filepath.Join(state, name)); !os.IsNotExist(err) {
			t.Fatalf("first review created %s: %v", name, err)
		}
	}
}

func TestFreshMissingAgentAndLogin(t *testing.T) {
	for _, kind := range []string{"missing-agent", "missing-login"} {
		t.Run(kind, func(t *testing.T) {
			bin, repo, state := fixture(t)
			git(t, repo, "checkout", "-b", "feature")
			os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
			git(t, repo, "add", "sample.txt")
			git(t, repo, "-c", "user.name=Test", "-c", "user.email=t@example.invalid", "commit", "-m", "change")
			agents := filepath.Join(filepath.Dir(state), "bin")
			os.Mkdir(agents, 0700)
			if kind == "missing-login" {
				os.WriteFile(filepath.Join(agents, "pi"), []byte("#!/bin/sh\nprintf 'No API key found for native provider\\n' >&2\nexit 1\n"), 0700)
			}
			env := []string{"PATH=" + agents + ":/usr/bin:/bin"}
			v := axi(t, bin, repo, state, `{"version":1,"op":"attach"}`, env...)
			q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": v["gate_id"], "action_id": "prepare-native", "intent": "Add sample", "target": "main"}
			b, _ := json.Marshal(q)
			v = axi(t, bin, repo, state, string(b), env...)
			if kind == "missing-agent" {
				if v["outcome"] != "refused" || !strings.Contains(v["error"].(string), "install pi") {
					t.Fatalf("missing-agent action: %v", v)
				}
				return
			}
			if v["outcome"] != "ok" {
				t.Fatal(v)
			}
			v = mutate(t, bin, repo, state, v, "resolve-rules", "load", nil)
			mutate(t, bin, repo, state, v, "review", "review", nil)
			run := runOf(t, waitReview(t, bin, repo, state))
			if run["status"] != "paused" {
				t.Fatalf("missing login: %v", run)
			}
			pause := run["pause"].(map[string]any)
			if !strings.Contains(pause["message"].(string), "/login") {
				t.Fatalf("missing native login action: %v", pause)
			}
		})
	}
}

func TestNativeClaudeAndCodexDefaultModels(t *testing.T) {
	for _, backend := range []string{backendClaude, backendCodex} {
		t.Run(backend+"-review", func(t *testing.T) {
			bin, repo, state, root, v := onePassWith(t, backend, "")
			mutate(t, bin, repo, state, v, "review", "native-default", nil)
			v = waitReview(t, bin, repo, state)
			if runOf(t, v)["status"] != "clean-review" {
				t.Fatal(v)
			}
			calls := agentCalls(t, root, backend)
			if len(calls) != 1 || has(strs(calls[0]["args"]), "--model") || has(strs(calls[0]["args"]), "-m") {
				t.Fatalf("native default was overridden: %v", calls)
			}
		})
		t.Run(backend+"-repair", func(t *testing.T) {
			bin, repo, state, root, v := pipelineWith(t, backendPi, "fixture/model", backend, "")
			os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
			os.WriteFile(filepath.Join(root, "edit-"+backend), []byte("repaired\n"), 0600)
			mutate(t, bin, repo, state, v, "pipeline", "native-fixer-default", nil)
			v = waitPipeline(t, bin, repo, state)
			if runOf(t, v)["status"] != "locally-ready" {
				t.Fatal(v)
			}
			calls := agentCalls(t, root, backend)
			if len(calls) != 1 || has(strs(calls[0]["args"]), "--model") || has(strs(calls[0]["args"]), "-m") {
				t.Fatalf("native fixer default was overridden: %v", calls)
			}
		})
	}
}

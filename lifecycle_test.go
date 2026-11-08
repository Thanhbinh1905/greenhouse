package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func runOf(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	r, ok := v["run"].(map[string]any)
	if !ok {
		t.Fatalf("no run: %v", v)
	}
	return r
}
func mutate(t *testing.T, bin, repo, home string, v map[string]any, op, action string, s any) map[string]any {
	t.Helper()
	q := map[string]any{"version": 1, "op": op, "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": action, "reason": "test adjustment"}
	if s != nil {
		q["settings"] = s
	}
	b, _ := json.Marshal(q)
	return axi(t, bin, repo, home, string(b))
}
func TestLifecycleSettingsAndIdentity(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	initial := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if runOf(t, initial)["status"] != "unconfigured" {
		t.Fatal(initial)
	}
	alias := filepath.Join(filepath.Dir(repo), "alias")
	if e := os.Symlink(repo, alias); e != nil {
		t.Fatal(e)
	}
	aliased := axi(t, bin, alias, home, `{"version":1,"op":"attach"}`)
	if aliased["run_id"] != initial["run_id"] {
		t.Fatal("alias split")
	}
	wt := filepath.Join(filepath.Dir(repo), "linked")
	git(t, repo, "worktree", "add", "-b", "linked", wt)
	linked := axi(t, bin, wt, home, `{"version":1,"op":"attach"}`)
	if linked["run_id"] == initial["run_id"] {
		t.Fatal("worktrees conflated")
	}
	s := map[string]any{"mode": "ask", "preset": "full", "limit": 7, "rules": []string{"unslop"}}
	applied := mutate(t, bin, repo, home, initial, "apply", "apply-1", s)
	if applied["outcome"] != "ok" {
		t.Fatal(applied)
	}
	again := mutate(t, bin, repo, home, initial, "apply", "apply-1", s)
	if !reflect.DeepEqual(applied, again) {
		t.Fatal("replay result changed")
	}
	reused := mutate(t, bin, repo, home, initial, "apply", "apply-1", initial["defaults"])
	if reused["outcome"] != "refused" {
		t.Fatal("action reused")
	}
	stale := mutate(t, bin, repo, home, initial, "apply", "apply-2", s)
	if stale["outcome"] != "refused" {
		t.Fatal("stale allowed")
	}
	reattach := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if !reflect.DeepEqual(runOf(t, reattach), runOf(t, applied)) {
		t.Fatal("attach changed run")
	}
	configs := runOf(t, reattach)["configs"].([]any)
	if len(configs) != 2 || configs[0].(map[string]any)["settings"].(map[string]any)["mode"] != "auto" {
		t.Fatal("immutable config changed")
	}
	fresh := mutate(t, bin, repo, home, reattach, "new", "new-1", nil)
	if fresh["run_id"] == initial["run_id"] {
		t.Fatal("new reused")
	}
	if runOf(t, fresh)["configs"].([]any)[0].(map[string]any)["settings"].(map[string]any)["mode"] != "auto" {
		t.Fatal("apply changed defaults")
	}
	saved := mutate(t, bin, repo, home, fresh, "save", "save-1", s)
	newSaved := mutate(t, bin, repo, home, saved, "new", "new-2", nil)
	if runOf(t, newSaved)["configs"].([]any)[0].(map[string]any)["settings"].(map[string]any)["mode"] != "ask" {
		t.Fatal("save not inherited")
	}
	os.WriteFile(filepath.Join(repo, "dirty"), []byte("preserve"), 0600)
	before := git(t, repo, "status", "--porcelain=v1")
	refs := git(t, repo, "show-ref")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "unpublished")
	movedRefs := git(t, repo, "show-ref")
	moved := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if moved["run_id"] != newSaved["run_id"] {
		t.Fatal("movement implicitly replaced")
	}
	if mutate(t, bin, repo, home, moved, "apply", "moved", s)["outcome"] != "refused" {
		t.Fatal("source movement allowed")
	}
	for _, op := range []string{"review", "approve", "repair", "recover", "cancel", "push"} {
		if axi(t, bin, repo, home, fmt.Sprintf(`{"version":1,"op":%q}`, op))["outcome"] != "refused" {
			t.Fatal(op)
		}
	}
	if got := git(t, repo, "status", "--porcelain=v1"); got != before {
		t.Fatal("dirty changed")
	}
	if git(t, repo, "show-ref") == refs {
		t.Fatal("fixture failed to move head")
	}
	if git(t, repo, "show-ref") != movedRefs {
		t.Fatal("product changed refs")
	}
	content, err := os.ReadFile(filepath.Join(repo, "dirty"))
	if err != nil || string(content) != "preserve" {
		t.Fatal("dirty content changed")
	}
	b, _ := os.ReadFile(filepath.Join(home, "state.json"))
	var store Store
	if e := json.Unmarshal(b, &store); e != nil {
		t.Fatal(e)
	}
	if len(store.Runs) != 4 || store.Runs[initial["run_id"].(string)].Submitted.Head != runOf(t, initial)["candidate"] {
		t.Fatal("old evidence lost")
	}
}

func stopDaemon(t *testing.T, home string) {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(home, "pid"))
	if e != nil {
		t.Fatal(e)
	}
	var pid int
	json.Unmarshal(b, &pid)
	if e = syscall.Kill(pid, syscall.SIGTERM); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, e := net.DialTimeout("unix", filepath.Join(home, "socket"), 50*time.Millisecond)
		if e != nil {
			return
		}
		c.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("daemon did not stop")
}
func TestRestartAndUnknownWriter(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	c, e := net.Dial("unix", filepath.Join(home, "socket"))
	if e != nil {
		t.Fatal(e)
	}
	c.Close()
	duplicate := exec.Command(bin, "daemon")
	duplicate.Env = append(os.Environ(), "GREENHOUSE_HOME="+home)
	if b, e := duplicate.CombinedOutput(); e == nil || !strings.Contains(string(b), "already owns") {
		t.Fatalf("singleton: %s %v", b, e)
	}
	if got := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`); !reflect.DeepEqual(got, v) {
		t.Fatal("second daemon changed listener/state")
	}
	stopDaemon(t, home)
	if got := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`); !reflect.DeepEqual(got, v) {
		t.Fatal("restart changed parked evidence")
	}
	stopDaemon(t, home)
	b, _ := os.ReadFile(filepath.Join(home, "state.json"))
	var s Store
	json.Unmarshal(b, &s)
	r := s.Runs[v["run_id"].(string)]
	r.Status = "writing"
	r.ReviewUsed = 2
	r.RepairUsed = 1
	if e = persist(home, &s); e != nil {
		t.Fatal(e)
	}
	recovered := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	rr := runOf(t, recovered)
	if rr["status"] != "recovery-required" || rr["review_used"] != float64(2) || rr["repair_used"] != float64(1) {
		t.Fatal(recovered)
	}
	for _, action := range recovered["actions"].([]any) {
		if action == "new" || action == "apply" {
			t.Fatal("unsafe action advertised")
		}
	}
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "moved while parked")
	recovered = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if mutate(t, bin, repo, home, recovered, "new", "unsafe", nil)["outcome"] != "refused" {
		t.Fatal("unknown writer replaced")
	}
	if mutate(t, bin, repo, home, recovered, "apply", "unsafe-apply", map[string]any{"mode": "auto", "preset": "full", "limit": 9})["outcome"] != "refused" {
		t.Fatal("unknown outcome mutated")
	}
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewRunAfterSourceMovement(t *testing.T) {
	bin, repo, home := fixture(t)
	initial := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	applied := mutate(t, bin, repo, home, initial, "apply", "settings", Settings{"ask", "full", 9, []string{"unslop"}})
	old := runOf(t, applied)
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "next")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	attached := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	if attached["run_id"] != initial["run_id"] || !reflect.DeepEqual(old, runOf(t, attached)) {
		t.Fatal("source movement changed retained run")
	}
	settings := Settings{"auto", "one-pass", 6, []string{"no-comments"}}
	for _, op := range []string{"apply", "save"} {
		if mutate(t, bin, repo, home, attached, op, op+"-moved", settings)["outcome"] != "refused" {
			t.Fatal("stale advancement allowed")
		}
	}
	if mutate(t, bin, repo, home, initial, "new", "stale-new", nil)["outcome"] != "refused" {
		t.Fatal("stale token replaced run")
	}
	next := mutate(t, bin, repo, home, attached, "new", "new-after-move", nil)
	if next["outcome"] != "ok" {
		t.Fatalf("safe New refused after source movement: %v", next)
	}
	var advertised bool
	for _, action := range attached["actions"].([]any) {
		if action == "new" {
			advertised = true
		}
		if action == "apply" || action == "save" {
			t.Fatal("stale advancement advertised")
		}
	}
	if !advertised {
		t.Fatal("safe New not advertised")
	}
	if next["run_id"] == initial["run_id"] || runOf(t, next)["candidate"] != head {
		t.Fatal("new source identity not captured")
	}
	if config := runOf(t, next)["configs"].([]any)[0].(map[string]any)["settings"].(map[string]any); config["mode"] != "auto" {
		t.Fatal("new inherited unsaved settings")
	}
	replay := mutate(t, bin, repo, home, attached, "new", "new-after-move", nil)
	if !reflect.DeepEqual(next, replay) {
		t.Fatal("new replay changed result")
	}
	b, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err = json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state["runs"].(map[string]any)[initial["run_id"].(string)], old) {
		t.Fatal("prior evidence changed on New")
	}
	git(t, repo, "checkout", "-b", "other")
	attached = axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	branched := mutate(t, bin, repo, home, attached, "new", "branch-new", nil)
	if branched["outcome"] != "ok" || runOf(t, branched)["submitted"].(map[string]any)["branch"] != "refs/heads/other" {
		t.Fatal("new branch identity not captured")
	}
	stopDaemon(t, home)
	if got := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`); !reflect.DeepEqual(got, branched) {
		t.Fatal("replacement not durable")
	}
}

func TestDaemonGitEnvironmentDoesNotSelectOtherRepository(t *testing.T) {
	bin, a, home := fixture(t)
	b := filepath.Join(filepath.Dir(a), "other")
	if err := os.Mkdir(b, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, b, "init", "-b", "other")
	git(t, b, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "other")
	linked := filepath.Join(filepath.Dir(a), "linked")
	git(t, a, "worktree", "add", "-b", "linked", linked)
	polluted := []string{"GIT_DIR=" + filepath.Join(a, ".git"), "GIT_WORK_TREE=" + a, "GIT_COMMON_DIR=" + filepath.Join(a, ".git"), "GIT_NAMESPACE=foreign", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.worktree", "GIT_CONFIG_VALUE_0=" + a}
	first := axi(t, bin, a, home, `{"version":1,"op":"attach"}`, polluted...)
	second := axi(t, bin, b, home, `{"version":1,"op":"attach"}`)
	if first["run_id"] == second["run_id"] {
		t.Fatalf("repository B attached to A's run: %v", second)
	}
	if submitted := runOf(t, second)["submitted"].(map[string]any); submitted["worktree"] != b || submitted["branch"] != "refs/heads/other" {
		t.Fatalf("wrong B identity: %v", submitted)
	}
	third := axi(t, bin, linked, home, `{"version":1,"op":"attach"}`, polluted...)
	if third["run_id"] == first["run_id"] || third["run_id"] == second["run_id"] {
		t.Fatalf("linked worktree conflated: %v", third)
	}
	if submitted := runOf(t, third)["submitted"].(map[string]any); submitted["worktree"] != linked || submitted["branch"] != "refs/heads/linked" {
		t.Fatalf("wrong worktree identity: %v", submitted)
	}
	stopDaemon(t, home)
	after := axi(t, bin, b, home, `{"version":1,"op":"attach"}`, polluted...)
	if after["run_id"] != second["run_id"] {
		t.Fatalf("restart changed B identity: %v", after)
	}
}

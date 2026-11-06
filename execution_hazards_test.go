package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReviewMovementAndUnknownRestart(t *testing.T) {
	for _, scenario := range []string{"movement", "restart"} {
		t.Run(scenario, func(t *testing.T) {
			bin, repo, home, v := prepareFixture(t)
			invocation := installFake(t, home, "while not os.path.exists(root+'/release'): time.sleep(0.01)\nwith open(root+'/released','w') as f: f.write('released')")
			v = mutate(t, bin, repo, home, v, "review", "start", nil)
			deadline := time.Now().Add(10 * time.Second)
			for {
				if b, _ := os.ReadFile(invocation); len(b) > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("executor not started")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if scenario == "movement" {
				git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "moving source")
				os.WriteFile(filepath.Join(filepath.Dir(home), "release"), []byte("release"), 0600)
				v = waitReview(t, bin, repo, home)
				if runOf(t, v)["status"] != "stale" {
					t.Fatal(v)
				}
			} else {
				pidBytes, err := os.ReadFile(filepath.Join(home, "pid"))
				if err != nil {
					t.Fatal(err)
				}
				var pid int
				if json.Unmarshal(pidBytes, &pid) != nil || pid <= 0 {
					t.Fatal("invalid fixture daemon identity")
				}
				if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
					t.Fatal(err)
				}
				lock, err := os.OpenFile(filepath.Join(home, "lock"), os.O_RDWR, 0600)
				if err != nil {
					t.Fatal(err)
				}
				for syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
					if time.Now().After(deadline) {
						t.Fatal("fixture daemon retained lock")
					}
					time.Sleep(time.Millisecond)
				}
				syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
				lock.Close()
				v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
				if runOf(t, v)["status"] != "recovery-required" {
					t.Fatal(v)
				}
				if mutate(t, bin, repo, home, v, "review", "retry", nil)["outcome"] != "refused" {
					t.Fatal("unknown outcome retried")
				}
			}
			os.WriteFile(filepath.Join(filepath.Dir(home), "release"), []byte("release"), 0600)
			for {
				if _, err := os.Stat(filepath.Join(filepath.Dir(home), "released")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("fixture executor did not release")
				}
				time.Sleep(time.Millisecond)
			}
			count, _ := os.ReadFile(invocation)
			if strings.Count(string(count), "invoked") != 1 {
				t.Fatal("duplicate execution")
			}
		})
	}
}
func TestRequiredRulesAndBudgetRefuseDispatch(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	invocation := installFake(t, home, "")
	settings := map[string]any{"mode": "auto", "preset": "one-pass", "limit": 1, "rules": []string{"missing"}}
	v = mutate(t, bin, repo, home, v, "apply", "missing", settings)
	v = mutate(t, bin, repo, home, v, "resolve-rules", "failed-rule", nil)
	refused := mutate(t, bin, repo, home, v, "review", "blocked", nil)
	if refused["outcome"] != "refused" || !strings.Contains(refused["error"].(string), "required rules") {
		t.Fatal(refused)
	}
	if _, err := os.Stat(invocation); !os.IsNotExist(err) {
		t.Fatal("blocked rule invoked executor")
	}
	stopDaemon(t, home)
	b, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Store
	json.Unmarshal(b, &s)
	r := s.Runs[v["run_id"].(string)]
	r.ReviewUsed = 1
	b, _ = json.Marshal(s)
	os.WriteFile(filepath.Join(home, "state.json"), b, 0600)
	v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	refused = mutate(t, bin, repo, home, v, "review", "cap", nil)
	if refused["outcome"] != "refused" || refused["error"] != "review budget exhausted" {
		t.Fatal(refused)
	}
	if _, err := os.Stat(invocation); !os.IsNotExist(err) {
		t.Fatal("exhausted cap invoked executor")
	}
}

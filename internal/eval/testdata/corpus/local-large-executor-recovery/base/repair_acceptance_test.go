package main

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFixerConfig(t *testing.T, home string, cfg FixerConfig) {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "fixer.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSelectedRepairRequiresConfiguredCheck(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := repairFixture(t)
	var cfg FixerConfig
	data, err := os.ReadFile(filepath.Join(home, "fixer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Checks = nil
	writeFixerConfig(t, home, cfg)
	response := repairRequest(t, bin, repo, home, v, "repair", "repair-without-check", selectedID(t, v))
	if response["outcome"] != "refused" || runOf(t, response)["repair_used"] != float64(0) {
		t.Fatalf("repair without required checks was admitted: %v", response)
	}
	if count, err := os.ReadFile(filepath.Join(filepath.Dir(home), "fix-invocations")); err == nil && len(count) > 0 {
		t.Fatalf("fixer ran without a required check: %q", count)
	}
}

func TestSelectedRepairRefusesChangedCheckAuthority(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := repairFixture(t)
	check := filepath.Join(filepath.Dir(home), "trusted-check.py")
	checkLog := filepath.Join(filepath.Dir(home), "check-ran")
	if err := os.WriteFile(check, []byte("#!/usr/bin/python3\nopen("+quotePython(checkLog)+", 'a').write('ran\\n')\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var cfg FixerConfig
	data, err := os.ReadFile(filepath.Join(home, "fixer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	fixer := filepath.Join(filepath.Dir(home), "change-check.py")
	program := "#!/usr/bin/python3\nimport pathlib\npathlib.Path('sample.txt').write_text('repaired\\n')\npathlib.Path(" + quotePython(check) + ").write_text('#!/usr/bin/python3\\nraise SystemExit(99)\\n')\n"
	if err := os.WriteFile(fixer, []byte(program), 0700); err != nil {
		t.Fatal(err)
	}
	cfg.Executable = fixer
	cfg.Checks = [][]string{{check}}
	writeFixerConfig(t, home, cfg)
	response := repairRequest(t, bin, repo, home, v, "repair", "changed-check-authority", selectedID(t, v))
	if response["outcome"] != "ok" {
		t.Fatal(response)
	}
	done := waitRepair(t, bin, repo, home)
	repair := runOf(t, done)["repair"].(map[string]any)
	if repair["status"] != "authority-changed" || repair["head"] != "" || runOf(t, done)["repair_used"] != float64(1) {
		t.Fatalf("changed check authority was not refused with consumed budget: %v", done)
	}
	if _, err := os.Stat(checkLog); !os.IsNotExist(err) {
		t.Fatalf("changed check executable ran: %v", err)
	}
	if strings.TrimSpace(git(t, repair["worktree"].(string), "rev-parse", "HEAD")) != repair["base"] {
		t.Fatal("authority change created a repair commit")
	}
}

func TestManagedRepairRefusesGitHookAndSigningHazards(t *testing.T) {
	t.Parallel()
	for _, hazard := range []string{"hook", "signing"} {
		t.Run(hazard, func(t *testing.T) {
			bin, repo, home, v := repairFixture(t)
			marker := filepath.Join(filepath.Dir(home), "hook-ran")
			if hazard == "hook" {
				hooks := strings.TrimSpace(git(t, repo, "rev-parse", "--path-format=absolute", "--git-path", "hooks"))
				if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nprintf ran > "+marker+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				git(t, repo, "config", "commit.gpgsign", "true")
			}
			response := repairRequest(t, bin, repo, home, v, "repair", "hazard-"+hazard, selectedID(t, v))
			if response["outcome"] != "refused" || runOf(t, response)["repair_used"] != float64(0) {
				t.Fatalf("managed repair did not refuse %s hazard: %v", hazard, response)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("repository hook executed: %v", err)
			}
			if count, err := os.ReadFile(filepath.Join(filepath.Dir(home), "fix-invocations")); err == nil && len(count) > 0 {
				t.Fatalf("fixer ran despite %s hazard: %q", hazard, count)
			}
		})
	}
}

func TestDaemonCrashAfterRepairCommitPreservesUnknownOutcome(t *testing.T) {
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	wrapperDir, err := os.MkdirTemp(os.Getenv("TMPDIR"), "repair-crash-git-")
	if err != nil {
		t.Fatal(err)
	}
	bin, repo, home, v := repairFixture(t)
	stopDaemon(t, home)
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	log := filepath.Join(filepath.Dir(home), "commit-wrapper-runs")
	checkLog := filepath.Join(filepath.Dir(home), "crash-check-runs")
	check := filepath.Join(filepath.Dir(home), "crash-check.py")
	if err := os.WriteFile(check, []byte("#!/usr/bin/python3\nopen("+quotePython(checkLog)+", 'a').write('check\\n')\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var cfg FixerConfig
	configData, err := os.ReadFile(filepath.Join(home, "fixer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(configData, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Checks = [][]string{{check}}
	writeFixerConfig(t, home, cfg)
	wrapper := `#!/usr/bin/python3
import json,os,subprocess,sys
real=sys.argv[1]
args=sys.argv[1:]
p=subprocess.run([real]+args)
if p.returncode: sys.exit(p.returncode)
if 'commit' in args:
    target=''
    for i,arg in enumerate(args[:-1]):
        if arg == '-C': target=args[i+1]
    if '/candidates/' in target:
        open(` + quotePython(log) + `,'a').write('commit\n')
        home=os.environ['GREENHOUSE_HOME']
        pid=json.load(open(os.path.join(home,'pid')))
        os.kill(pid,9)
sys.exit(p.returncode)
`
	if err := os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(strings.Replace(wrapper, "real=sys.argv[1]", "real="+quotePython(realGit), 1)), 0700); err != nil {
		t.Fatal(err)
	}
	v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	admitted := repairRequest(t, bin, repo, home, v, "repair", "crash-after-commit", selectedID(t, v))
	if admitted["outcome"] != "ok" {
		t.Fatal(admitted)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if data, readErr := os.ReadFile(log); readErr == nil && strings.Count(string(data), "commit\n") == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if data, readErr := os.ReadFile(log); readErr != nil || strings.Count(string(data), "commit\n") != 1 {
		state := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
		t.Fatalf("fixture did not observe the candidate commit: %q %v state=%v", data, readErr, state)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("unix", filepath.Join(home, "socket"), 20*time.Millisecond)
		if dialErr != nil {
			break
		}
		conn.Close()
		time.Sleep(10 * time.Millisecond)
	}
	recovered := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	run := runOf(t, recovered)
	repair := run["repair"].(map[string]any)
	if run["status"] != "recovery-required" || repair["status"] != "unknown-outcome" || repair["writer"] != "unknown" || run["repair_used"] != float64(1) {
		t.Fatalf("interrupted post-commit outcome was not parked with budget retained: %v", recovered)
	}
	candidate := repair["worktree"].(string)
	if strings.TrimSpace(git(t, candidate, "rev-parse", "HEAD")) == repair["base"] || strings.TrimSpace(git(t, candidate, "rev-parse", "HEAD^")) != repair["base"] {
		t.Fatal("post-commit repair candidate ref or lineage was not preserved")
	}
	count, err := os.ReadFile(filepath.Join(filepath.Dir(home), "fix-invocations"))
	if err != nil || strings.Count(string(count), "fix\n") != 1 {
		t.Fatalf("restart duplicated fixer execution: %q %v", count, err)
	}
	commits, err := os.ReadFile(log)
	if err != nil || strings.Count(string(commits), "commit\n") != 1 {
		t.Fatalf("restart duplicated commit execution: %q %v", commits, err)
	}
	checks, err := os.ReadFile(checkLog)
	if err != nil || strings.Count(string(checks), "check\n") != 1 {
		t.Fatalf("restart duplicated check execution: %q %v", checks, err)
	}
}

func quotePython(s string) string { return "'" + strings.ReplaceAll(s, "'", "\\'") + "'" }

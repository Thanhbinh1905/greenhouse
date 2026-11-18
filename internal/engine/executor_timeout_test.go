package engine

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

const partialQuotaThenSleep = `print('{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"429: quota exhausted"}}', flush=True)
time.sleep(30)
`

func TestConfiguredReviewExecutorTimeout(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, partialQuotaThenSleep)
	settings := runOf(t, v)["configs"].([]any)
	config := settings[len(settings)-1].(map[string]any)["settings"].(map[string]any)
	config["executor_timeout"] = "1s"
	v = mutate(t, bin, repo, home, v, "apply", "short-timeout", config)
	if v["outcome"] != "ok" {
		t.Fatalf("configured timeout rejected: %v", v)
	}
	v = mutate(t, bin, repo, home, v, "resolve-rules", "timeout-rules", nil)
	v = mutate(t, bin, repo, home, v, "review", "start", nil)
	if v["outcome"] != "ok" {
		t.Fatalf("review refused: %v", v)
	}
	v = waitReview(t, bin, repo, home)
	review := runOf(t, v)["review"].(map[string]any)
	if review["result"] != nil || runOf(t, v)["pause"] != nil || review["status"] == "paused" {
		t.Fatalf("timed-out review status=%v pause=%v", review["status"], runOf(t, v)["pause"])
	}
	records, err := filepath.Glob(filepath.Join(home, "executors", "*.json"))
	if err != nil || len(records) == 0 {
		t.Fatalf("no executor records: %v %v", records, err)
	}
	data, err := os.ReadFile(records[0])
	if err != nil {
		t.Fatal(err)
	}
	var record runner.ExecutorRecord
	if err := json.Unmarshal(data, &record); err != nil || !record.Stopped || !record.Reaped {
		t.Fatalf("executor did not stop and reap: %s (%v)", data, err)
	}
	t.Logf("stopped executor evidence: %s", data)
}

func TestExecutorTimeoutSettings(t *testing.T) {
	store, err := load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := executorTimeout(store.Defaults); got != 45*time.Minute {
		t.Fatalf("default timeout: %v", got)
	}
	for _, value := range []string{"45m", "1s", "90m", "250ms"} {
		settings := store.Defaults
		settings.ExecutorTimeout = executorDuration(value)
		if !valid(settings) {
			t.Errorf("valid duration rejected: %s", value)
		}
	}
	for _, value := range []string{"0", "0s", "-1m", "45", "forever", "999999999999999999999h"} {
		settings := store.Defaults
		settings.ExecutorTimeout = executorDuration(value)
		if valid(settings) {
			t.Errorf("invalid duration accepted: %s", value)
		}
	}
	if executorTimeout(Settings{}) != 45*time.Minute {
		t.Fatal("legacy settings lost default timeout")
	}
}

func TestConfiguredFixerExecutorTimeout(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	configs := runOf(t, v)["configs"].([]any)
	settings := configs[len(configs)-1].(map[string]any)["settings"].(map[string]any)
	settings["executor_timeout"] = "2s"
	v = mutate(t, bin, repo, home, v, "apply", "fixer-timeout", settings)
	v = mutate(t, bin, repo, home, v, "resolve-rules", "fixer-rules", nil)
	v = mutate(t, bin, repo, home, v, "review", "review", nil)
	v = waitReview(t, bin, repo, home)
	if runOf(t, v)["status"] != "outstanding-findings" {
		t.Fatal(v)
	}
	fake := filepath.Join(filepath.Dir(home), "sleeping-fixer")
	if err := os.WriteFile(fake, []byte("#!/usr/bin/python3\nimport time\n"+partialQuotaThenSleep), 0700); err != nil {
		t.Fatal(err)
	}
	config := FixerConfig{Executable: fake, Model: "fixture/model", Name: "Test", Email: "test@example.invalid", Checks: [][]string{{"/bin/true"}}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConfigBytes(filepath.Join(home, "fixer.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	v = repairRequest(t, bin, repo, home, v, "repair", "repair", selectedID(t, v))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	v = waitRepair(t, bin, repo, home)
	repair := runOf(t, v)["repair"].(map[string]any)
	if repair["status"] != "fixer-failed" || repair["failure"] != nil || runOf(t, v)["pause"] != nil {
		t.Fatalf("timed-out fixer status=%v failure=%v pause=%v", repair["status"], repair["failure"], runOf(t, v)["pause"])
	}

	records, err := filepath.Glob(filepath.Join(home, "executors", repair["id"].(string)+".*.json"))
	if err != nil || len(records) != 1 {
		t.Fatalf("executor records: %v %v; repair=%v", records, err, repair)
	}
	data, err = os.ReadFile(records[0])
	if err != nil {
		t.Fatal(err)
	}
	var record runner.ExecutorRecord
	if err := json.Unmarshal(data, &record); err != nil || !record.Stopped || !record.Reaped {
		t.Fatalf("fixer did not stop and reap: %s (%v)", data, err)
	}
	t.Logf("stopped fixer evidence: %s", data)
}

func TestExecutorTimeoutSavedAndInherited(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	configs := runOf(t, v)["configs"].([]any)
	settings := configs[len(configs)-1].(map[string]any)["settings"].(map[string]any)
	settings["executor_timeout"] = "90m"
	v = mutate(t, bin, repo, home, v, "apply", "long-timeout", settings)
	delete(settings, "executor_timeout")
	v = mutate(t, bin, repo, home, v, "apply", "inherit-timeout", settings)
	configs = runOf(t, v)["configs"].([]any)
	if got := configs[len(configs)-1].(map[string]any)["settings"].(map[string]any)["executor_timeout"]; got != "90m" {
		t.Fatalf("timeout was not inherited: %v", got)
	}
	settings["executor_timeout"] = "1h"
	v = mutate(t, bin, repo, home, v, "save", "save-timeout", settings)
	if v["outcome"] != "ok" || v["defaults"].(map[string]any)["executor_timeout"] != "1h" {
		t.Fatalf("timeout was not saved: %v", v)
	}
	killRepairDaemon(t, home)
	v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	if v["defaults"].(map[string]any)["executor_timeout"] != "1h" {
		t.Fatalf("saved timeout lost after restart: %v", v)
	}
}

func TestExecutorTimeoutRepositoryOverride(t *testing.T) {
	store, err := load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings, _, err := parseOverride([]byte(`{"version":1,"executor_timeout":"90m"}`), store.Defaults)
	if err != nil || executorTimeout(settings) != 90*time.Minute {
		t.Fatalf("timeout override: %v %v", settings, err)
	}
	if _, _, err := parseOverride([]byte(`{"version":1,"executor_timeout":"-1s"}`), store.Defaults); err == nil {
		t.Fatal("invalid override accepted")
	}
}

func TestConfiguredReviewDeadlineExceedsTenMinutes(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `with open('/proc/'+str(os.getppid())+'/cmdline','rb') as f: args=f.read().split(b'\x00')
with open(root+'/executor-deadline','w') as f: f.write(args[3].decode())`)
	configs := runOf(t, v)["configs"].([]any)
	settings := configs[len(configs)-1].(map[string]any)["settings"].(map[string]any)
	settings["executor_timeout"] = "1h"
	v = mutate(t, bin, repo, home, v, "apply", "long-review-timeout", settings)
	v = mutate(t, bin, repo, home, v, "resolve-rules", "long-review-rules", nil)
	started := time.Now()
	v = mutate(t, bin, repo, home, v, "review", "long-review", nil)
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	v = waitReview(t, bin, repo, home)
	if runOf(t, v)["status"] != "outstanding-findings" {
		t.Fatal(v)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(home), "executor-deadline"))
	if err != nil {
		t.Fatal(err)
	}
	deadline, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	remaining := time.Unix(deadline, 0).Sub(started)
	if remaining < time.Hour || remaining > time.Hour+time.Minute {
		t.Fatalf("configured hour did not reach review supervisor: %s", remaining)
	}
	t.Logf("review supervisor deadline is %s after launch", remaining)
}

func TestEmptyExecutorTimeoutOverrideInheritsUserValue(t *testing.T) {
	store, err := load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := store.Defaults
	base.ExecutorTimeout = "90m"
	settings, _, err := parseOverride([]byte(`{"version":1,"executor_timeout":""}`), base)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ExecutorTimeout != "90m" {
		t.Fatalf("empty override replaced user timeout: got %q, want 90m", settings.ExecutorTimeout)
	}
}

func TestExecutorTimeoutRejectsNonStringJSON(t *testing.T) {
	for _, value := range []string{"null", "123", "true", "[]", "{}"} {
		t.Run(value, func(t *testing.T) {
			var request Request
			if err := json.Unmarshal([]byte(`{"settings":{"executor_timeout":`+value+`}}`), &request); err == nil {
				t.Errorf("request accepted non-string timeout: %s", value)
			}
			store, err := load(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := parseOverride([]byte(`{"version":1,"executor_timeout":`+value+`}`), store.Defaults); err == nil {
				t.Errorf("override accepted non-string timeout: %s", value)
			}
		})
	}
}

func TestExecutorTimeoutNullRejectedThroughCLI(t *testing.T) {
	bin, repo, home := fixture(t)
	cmd := exec.Command(bin, "axi")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
	cmd.Stdin = strings.NewReader(`{"version":1,"op":"apply","settings":{"executor_timeout":null}}`)
	data, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(data), "executor_timeout must be a string") {
		t.Fatalf("CLI accepted null timeout: %s (%v)", data, err)
	}
}

func TestRestartAdoptsTimedOutReview(t *testing.T) {
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, `print('{"type":"message_end","message":{"role":"assistant","model":"model","usage":{"input":100,"output":20,"totalTokens":120}}}', flush=True)
`+partialQuotaThenSleep)
	configs := runOf(t, v)["configs"].([]any)
	settings := configs[len(configs)-1].(map[string]any)["settings"].(map[string]any)
	settings["executor_timeout"] = "1s"
	v = mutate(t, bin, repo, home, v, "apply", "timeout-settings", settings)
	v = mutate(t, bin, repo, home, v, "resolve-rules", "timeout-rules", nil)
	v = mutate(t, bin, repo, home, v, "review", "start-timeout", nil)
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	statePath := filepath.Join(home, "state.json")
	stateBeforeReviewCompletion, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	waitReview(t, bin, repo, home)
	stopDaemonAndWait(t, home)
	if err := os.WriteFile(statePath, stateBeforeReviewCompletion, 0600); err != nil {
		t.Fatal(err)
	}
	v = axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`)
	run := runOf(t, v)
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	var recovered Run
	if err := json.Unmarshal(data, &recovered); err != nil {
		t.Fatal(err)
	}
	recovered.Host = &Host{Name: "fixture", ID: "restart"}
	host := hostResult(&recovered, true)
	x := recovered.Review
	if x.Status != "cancelled" || x.Reason != "timeout" || !strings.Contains(x.Diagnostic, "1s") || host.Reason != "timeout" || host.Usage.Tokens != 120 || len(recovered.Attempts) != 1 || recovered.Attempts[0].Reason != "timeout" || recovered.Attempts[0].Usage.Tokens != 120 {
		t.Fatalf("restart lost timeout: run=%s review=%s reason=%q diagnostic=%q host reason=%q attempts=%+v recovery=%+v", recovered.Status, x.Status, x.Reason, x.Diagnostic, host.Reason, recovered.Attempts, recovered.Recovery)
	}
}

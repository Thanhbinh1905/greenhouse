package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func grantPlan(grants ...map[string]any) func(root string) map[string]any {
	return func(root string) map[string]any {
		p := repairPlan(root)
		if len(grants) > 0 {
			p["grants"] = grants
		}
		return p
	}
}

func TestGateRepairAddingFileNeedsExplicitRunGrant(t *testing.T) {
	t.Parallel()
	bin, repo, home, root, v := authorityFixture(t, "auto", 6, grantPlan())
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	os.WriteFile(filepath.Join(root, "fix-extra"), []byte("fixtures/added.txt"), 0600)
	submitted := runOf(t, v)["candidate"]
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	d := decisionIn(t, parked)
	if d["kind"] != "authority" || d["category"] != "add-files" || d["user_only"] != true || strings.Join(strs(d["options"]), ",") != "grant,rethink,discard" {
		t.Fatalf("adding a file did not park a grantable authority decision: %v", d)
	}
	run := runOf(t, parked)
	p := run["pipeline"].(map[string]any)
	repair := p["repairs"].([]any)[0].(map[string]any)
	if repair["status"] != "needs-authority" || repair["head"] != "" || run["candidate"] != submitted || !strings.Contains(repair["partial"].(string), "fixtures/added.txt") {
		t.Fatalf("ungranted addition was committed or lost: %v", repair)
	}
	if status := git(t, p["worktree"].(string), "status", "--porcelain"); status != "" {
		t.Fatalf("candidate not restored after the refused repair: %q", status)
	}
	granted := decide(t, bin, repo, home, parked, "grant-add", "grant", nil)
	if granted["outcome"] != "ok" {
		t.Fatalf("explicit grant refused: %v", granted)
	}
	done := waitPipeline(t, bin, repo, home)
	run = runOf(t, done)
	p = run["pipeline"].(map[string]any)
	if run["status"] != "locally-ready" {
		t.Fatalf("granted repair did not complete: %v", p)
	}
	grants := p["grants"].([]any)
	grant := grants[0].(map[string]any)
	if len(grants) != 1 || grant["authority"] != "add-files" || strings.Join(strs(grant["stages"]), ",") != "test" || strings.Join(strs(grant["paths"]), ",") != "fixtures/added.txt" || grant["source"] != "run" || grant["actor"] != "axi" || grant["decision"] != d["id"] {
		t.Fatalf("grant not scoped and audited: %v", grants)
	}
	last := p["repairs"].([]any)[len(p["repairs"].([]any))-1].(map[string]any)
	if strings.Join(strs(last["granted"]), ",") != "add-files:fixtures/added.txt" || last["head"] != run["candidate"] {
		t.Fatalf("repair does not record the granted change: %v", last)
	}
	if got := git(t, repo, "show", run["candidate"].(string)+":fixtures/added.txt"); got != "added\n" {
		t.Fatalf("granted file not committed: %q", got)
	}
	if !strings.Contains(string(mustRead(t, filepath.Join(root, "fix-input.json"))), `"Grants"`) {
		t.Fatal("fixer input does not state its grants")
	}
	found := false
	for _, e := range run["events"].([]any) {
		event := e.(map[string]any)
		found = found || event["kind"] == "grant" && event["actor"] == "axi"
	}
	if !found {
		t.Fatal("grant not recorded as an event")
	}
}

func TestTrustedConfigurationGrantIsScoped(t *testing.T) {
	t.Parallel()
	grant := map[string]any{"authority": "delete-files", "stages": []string{"test"}, "paths": []string{"README.md"}, "reason": "the test repair may remove the readme"}
	bin, repo, home, root, v := authorityFixture(t, "auto", 6, grantPlan(grant))
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	os.WriteFile(filepath.Join(root, "fix-remove"), []byte("README.md"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	done := waitPipeline(t, bin, repo, home)
	run := runOf(t, done)
	p := run["pipeline"].(map[string]any)
	if run["status"] != "locally-ready" {
		t.Fatalf("configured grant did not cover the deletion: %v", p)
	}
	grants := p["grants"].([]any)
	if len(grants) != 1 || grants[0].(map[string]any)["source"] != "pipeline.json" || grants[0].(map[string]any)["reason"] != grant["reason"] {
		t.Fatalf("configured grant not recorded: %v", grants)
	}
	repair := p["repairs"].([]any)[0].(map[string]any)
	if strings.Join(strs(repair["granted"]), ",") != "delete-files:README.md" {
		t.Fatalf("deletion not recorded against the grant: %v", repair)
	}
	if out := git(t, repo, "ls-tree", "--name-only", run["candidate"].(string)); strings.Contains(out, "README.md") {
		t.Fatal("granted deletion not committed")
	}
}

func TestGrantOutsideScopeStillParks(t *testing.T) {
	t.Parallel()
	grant := map[string]any{"authority": "delete-files", "stages": []string{"lint"}, "paths": []string{"README.md"}, "reason": "lint only"}
	bin, repo, home, root, v := authorityFixture(t, "auto", 6, grantPlan(grant))
	os.WriteFile(filepath.Join(root, "require-test"), []byte("repaired\n"), 0600)
	os.WriteFile(filepath.Join(root, "fix-remove"), []byte("README.md"), 0600)
	mutate(t, bin, repo, home, v, "pipeline", "start", nil)
	parked := waitPipeline(t, bin, repo, home)
	if d := decisionIn(t, parked); d["category"] != "destructive" || d["status"] != "open" {
		t.Fatalf("a grant for another stage covered the deletion: %v", d)
	}
}

func TestInvalidGrantConfigurationIsRefused(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "state")
	os.MkdirAll(home, 0700)
	t.Setenv("GREENHOUSE_HOME", home)
	writeExecutable(t, filepath.Join(root, "gate"), gateCommand)
	for name, grant := range map[string]map[string]any{
		"authority": {"authority": "execute", "stages": []string{"test"}, "paths": []string{"x"}, "reason": "r"},
		"stage":     {"authority": "add-files", "stages": []string{"push"}, "paths": []string{"x"}, "reason": "r"},
		"path":      {"authority": "add-files", "stages": []string{"test"}, "paths": []string{"../x"}, "reason": "r"},
		"reason":    {"authority": "add-files", "stages": []string{"test"}, "paths": []string{"x"}},
	} {
		writePrivateJSON(t, filepath.Join(home, "pipeline.json"), grantPlan(grant)(root))
		if _, _, err := trustedPlan(); err == nil || !strings.Contains(err.Error(), "grant") {
			t.Fatalf("%s: invalid grant accepted: %v", name, err)
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

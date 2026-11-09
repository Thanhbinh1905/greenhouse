package main

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

const probeScript = `
import json,os,socket,sys
def write(path):
 try:
  open(path,'a').write('escape')
  return 'allowed'
 except OSError as e:
  return 'denied'
def connect(port):
 s=socket.socket()
 s.settimeout(2)
 try:
  s.connect(('127.0.0.1',port))
  return 'allowed'
 except OSError as e:
  return 'denied'
 finally:
  s.close()
`

func fixtureProviderPort(t *testing.T, home string) int {
	t.Helper()
	var cfg TrustedProviderConfig
	b, _ := os.ReadFile(filepath.Join(home, "providers.json"))
	var fields map[string]json.RawMessage
	json.Unmarshal(b, &fields)
	json.Unmarshal(fields["providers"], &cfg.Providers)
	u, err := url.Parse(cfg.Providers["fixture"].BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	json.Unmarshal([]byte(u.Port()), &port)
	return port
}

func probePorts(t *testing.T, home string) (string, string) {
	t.Helper()
	blocked, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { blocked.Close() })
	go func() {
		for {
			c, err := blocked.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	allowed := fixtureProviderPort(t, home)
	return jsonText(blocked.Addr().(*net.TCPAddr).Port), jsonText(allowed)
}

func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func confinedFakes(t *testing.T, repo, home, probe, blocked, allowed, reviewerExtra string) {
	t.Helper()
	root := filepath.Dir(home)
	reviewer := `#!/usr/bin/python3
import sys
if '--list-models' in sys.argv:
 print('provider model context')
 print('fixture model 128K')
 sys.exit(0)
` + probeScript + `
b=json.load(sys.stdin)
cwd=os.getcwd()
result={'home':write(ROOT+'/review-escape'),'repo':write(REPO+'/review-escape'),'snapshot':write(cwd+'/sample.txt'),'blocked':connect(BLOCKED),'provider':connect(ALLOWED)}
EXTRA
json.dump(result,open(PROBE+'/review.json','w'))
v={'head':b['Head'],'verdict':'findings','coverage':b['Paths'],'findings':[{'id':'x','priority':'P2','origin':'correctness','file':'sample.txt','start':1,'end':1,'explanation':'Wording is wrong','action':'Rewrite the line','rule':'','head':b['Head']}]}
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps(v)}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
`
	fixer := `#!/usr/bin/python3
import sys
` + probeScript + `
b=json.load(sys.stdin)
result={'candidate':write('sample.txt'),'home':write(ROOT+'/fix-escape'),'repo':write(REPO+'/fix-escape'),'blocked':connect(BLOCKED)}
open('sample.txt','w').write('repaired\n')
json.dump(result,open(PROBE+'/fix.json','w'))
print(json.dumps({'type':'message_end','message':{'role':'assistant','provider':'fixture','model':'model','stopReason':'stop','content':[{'type':'text','text':json.dumps({'responses':[{'finding':f['id'],'response':'fixed','reason':'Rewrote the line'} for f in b.get('Selected',[])]})}]}}))
print(json.dumps({'type':'agent_settled','aborted':False}))
`
	for name, script := range map[string]string{"reviewer": reviewer, "fixer": fixer} {
		script = strings.NewReplacer("ROOT", jsonText(root), "REPO", jsonText(repo), "PROBE", jsonText(probe), "BLOCKED", blocked, "ALLOWED", allowed, "EXTRA", reviewerExtra).Replace(script)
		if err := os.WriteFile(filepath.Join(root, "confined-"+name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	writePrivateJSON(t, filepath.Join(home, "reviewer.json"), ReviewerConfig{Executable: filepath.Join(root, "confined-reviewer"), Model: "fixture/model"})
	writePrivateJSON(t, filepath.Join(home, "fixer.json"), map[string]any{"executable": filepath.Join(root, "confined-fixer"), "model": "fixture/model", "name": "Test", "email": "test@example.invalid", "checks": [][]string{{"/usr/bin/true"}}})
	writePrivateJSON(t, filepath.Join(home, "backends.json"), map[string]any{"writable": map[string][]string{"reviewer": {probe}, "fixer": {probe}}})
}

func reprepare(t *testing.T, bin, repo, home string, v map[string]any, axiCall func(string) map[string]any) map[string]any {
	t.Helper()
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": "none", "action_id": "reprepare", "intent": "Review sample change", "target": runOf(t, v)["target"]}
	v = axiCall(jsonText(q))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	return v
}

func readProbe(t *testing.T, path string) map[string]string {
	t.Helper()
	var out map[string]string
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("agent probe missing: %v", err)
	}
	json.Unmarshal(b, &out)
	return out
}

func TestReviewerAndFixerRunUnderOSConfinement(t *testing.T) {
	t.Parallel()
	if _, err := landlockABI(); err != nil {
		t.Skip("Landlock unavailable:", err)
	}
	bin, repo, home, v := prepareFixture(t)
	probe := filepath.Join(filepath.Dir(home), "probe")
	os.Mkdir(probe, 0700)
	blocked, allowed := probePorts(t, home)
	confinedFakes(t, repo, home, probe, blocked, allowed, "")
	v = reprepare(t, bin, repo, home, v, func(q string) map[string]any { return axi(t, bin, repo, home, q) })

	v = mutate(t, bin, repo, home, v, "review", "review", nil)
	v = waitReview(t, bin, repo, home)
	review := readProbe(t, filepath.Join(probe, "review.json"))
	want := map[string]string{"home": "denied", "repo": "denied", "snapshot": "denied", "blocked": "denied", "provider": "allowed"}
	for k, w := range want {
		if review[k] != w {
			t.Fatalf("reviewer access %s = %q, want %q (all: %v)", k, review[k], w, review)
		}
	}
	reviewID := runOf(t, v)["review"].(map[string]any)["id"].(string)
	for _, suffix := range []string{".lease", ".out", ".err"} {
		if _, err := os.Stat(filepath.Join(home, "executors", reviewID+".0"+suffix)); err != nil {
			t.Fatalf("the confined reviewer did not run inside the leased executor runner: %v", err)
		}
	}
	c := runOf(t, v)["review"].(map[string]any)["confinement"].(map[string]any)
	if c["status"] != "enforced" || c["role"] != "reviewer" || c["mechanism"] != "landlock" || len(c["writable"].([]any)) != 2 || !strings.Contains(jsonText(c["writable"]), jsonText(probe)) || strings.Contains(jsonText(c["writable"]), jsonText(repo)) || !strings.Contains(jsonText(c["tcp_connect_ports"]), allowed) || strings.Contains(jsonText(c["tcp_connect_ports"]), blocked) {
		t.Fatalf("reviewer confinement record = %v", c)
	}

	v = repairRequest(t, bin, repo, home, v, "repair", "repair", selectedID(t, v))
	if v["outcome"] != "ok" {
		t.Fatal(v)
	}
	v = waitRepair(t, bin, repo, home)
	fix := readProbe(t, filepath.Join(probe, "fix.json"))
	for k, w := range map[string]string{"candidate": "allowed", "home": "denied", "repo": "denied", "blocked": "denied"} {
		if fix[k] != w {
			t.Fatalf("fixer access %s = %q, want %q (all: %v)", k, fix[k], w, fix)
		}
	}
	repair := runOf(t, v)["repair"].(map[string]any)
	fc := repair["confinement"].(map[string]any)
	if fc["status"] != "enforced" || fc["role"] != "fixer" || !strings.Contains(jsonText(fc["writable"]), jsonText(repair["worktree"])) || strings.Contains(jsonText(fc["writable"]), jsonText(repo)) {
		t.Fatalf("fixer confinement record = %v", fc)
	}
	for _, name := range []string{"review-escape", "fix-escape"} {
		for _, dir := range []string{repo, filepath.Dir(home)} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				t.Fatalf("confined agent wrote %s", filepath.Join(dir, name))
			}
		}
	}
	screen := terminalOutput(t, bin, repo, home, "q\n")
	if !strings.Contains(screen, "Confinement: reviewer landlock enforced") || !strings.Contains(screen, "Confinement: fixer landlock enforced") {
		t.Fatalf("terminal must show confinement:\n%s", screen)
	}
}

func terminalOutput(t *testing.T, bin, repo, home, keys string) string {
	t.Helper()
	c := exec.Command(bin)
	c.Dir = repo
	c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "HOME="+filepath.Dir(home), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GREENHOUSE_TEST_PROVIDER_KEY=fixture-secret", "COLUMNS=100")
	c.Stdin = strings.NewReader(keys)
	b, _ := c.CombinedOutput()
	return string(b)
}

// Without Landlock the daemon must report a degraded state; seccomp makes
// landlock_create_ruleset fail in a child that then starts the real client.
func init() {
	argv := os.Getenv("GREENHOUSE_TEST_WITHOUT_LANDLOCK")
	if argv == "" {
		return
	}
	runtime.LockOSThread()
	var args []string
	json.Unmarshal([]byte(argv), &args)
	filter := []struct {
		code   uint16
		jt, jf uint8
		k      uint32
	}{
		{0x20, 0, 0, 0},
		{0x15, 0, 1, landlockCreateRuleset},
		{0x06, 0, 0, 0x00050000 | uint32(syscall.ENOSYS)},
		{0x06, 0, 0, 0x7fff0000},
	}
	prog := struct {
		len    uint16
		filter unsafe.Pointer
	}{uint16(len(filter)), unsafe.Pointer(&filter[0])}
	if _, _, e := syscall.RawSyscall6(syscall.SYS_PRCTL, prSetNoNewPriv, 1, 0, 0, 0, 0); e != 0 {
		os.Exit(90)
	}
	if _, _, e := syscall.RawSyscall(syscall.SYS_PRCTL, 22, 2, uintptr(unsafe.Pointer(&prog))); e != 0 {
		os.Exit(91)
	}
	syscall.Exec(args[0], args, os.Environ())
	os.Exit(92)
}

func axiWithoutLandlock(t *testing.T, bin, repo, home, request string) map[string]any {
	t.Helper()
	c := exec.Command(os.Args[0])
	c.Dir = repo
	c.Env = append(os.Environ(), "GREENHOUSE_TEST_WITHOUT_LANDLOCK="+jsonText([]string{bin, "axi"}), "GREENHOUSE_HOME="+home, "HOME="+filepath.Dir(home), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GREENHOUSE_TEST_PROVIDER_KEY=fixture-secret")
	c.Stdin = strings.NewReader(request)
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("axi without landlock: %v %s", err, b)
	}
	var r map[string]any
	if err = json.Unmarshal(b, &r); err != nil {
		t.Fatalf("json %s: %v", b, err)
	}
	return r
}

func TestUnavailableConfinementIsVisibleAndSnapshotTamperingDetected(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	stopDaemon(t, home)
	if v = axiWithoutLandlock(t, bin, repo, home, `{"version":1,"op":"inspect"}`); v["outcome"] != "ok" {
		t.Fatal(v)
	}
	probe := filepath.Join(filepath.Dir(home), "probe")
	os.Mkdir(probe, 0700)
	blocked, allowed := probePorts(t, home)
	confinedFakes(t, repo, home, probe, blocked, allowed, `
os.chmod(cwd,0o755)
os.chmod(cwd+'/sample.txt',0o644)
open(cwd+'/sample.txt','w').write('tampered\n')`)
	v = reprepare(t, bin, repo, home, v, func(q string) map[string]any { return axiWithoutLandlock(t, bin, repo, home, q) })
	v = mutate(t, bin, repo, home, v, "review", "review", nil)
	v = waitReview(t, bin, repo, home)
	review := runOf(t, v)["review"].(map[string]any)
	c := review["confinement"].(map[string]any)
	if c["status"] != "degraded" || c["network"] != "unrestricted" || !strings.Contains(c["reason"].(string), "Landlock unavailable") {
		t.Fatalf("missing degraded confinement record: %v", c)
	}
	snapshot := review["snapshot"].(map[string]any)
	if runOf(t, v)["status"] != "inconclusive" || !strings.HasPrefix(snapshot["check"].(string), "modified") || !strings.Contains(review["log"].(string), "candidate snapshot modified") {
		t.Fatalf("tampered snapshot must make the review inconclusive: status %v snapshot %v log %v", runOf(t, v)["status"], snapshot, review["log"])
	}
	if got := git(t, repo, "show", "HEAD:sample.txt"); got != "review me\n" {
		t.Fatalf("candidate changed: %q", got)
	}
	screen := terminalOutput(t, bin, repo, home, "q\n")
	if !strings.Contains(screen, "Confinement: reviewer DEGRADED") || !strings.Contains(screen, "Landlock unavailable") || !strings.Contains(screen, "Snapshot:") {
		t.Fatalf("terminal must show degraded confinement:\n%s", screen)
	}
}

func TestVendorReviewersCannotWriteBackendConfig(t *testing.T) {
	t.Parallel()
	if _, err := landlockABI(); err != nil {
		t.Skip("Landlock unavailable:", err)
	}
	for _, c := range []struct{ backend, model string }{{"claude", "claude-haiku-5-5"}, {"codex", "gpt-5.6-luna"}} {
		t.Run(c.backend, func(t *testing.T) {
			bin, repo, home, root, v := onePassWith(t, c.backend, c.model)
			probe := filepath.Join(root, "probe")
			os.Mkdir(probe, 0700)
			for _, dir := range []string{".claude", ".claude/hooks", ".codex"} {
				os.MkdirAll(filepath.Join(root, dir), 0700)
			}
			for _, file := range []string{".claude/.credentials.json", ".claude.json", ".claude/settings.json", ".codex/auth.json", ".codex/config.toml"} {
				os.WriteFile(filepath.Join(root, file), []byte("{}"), 0600)
			}
			os.WriteFile(filepath.Join(root, "probe-"+c.backend), []byte(probe), 0600)
			writePrivateJSON(t, filepath.Join(home, "backends.json"), map[string]any{
				"claude":   map[string]any{"executable": filepath.Join(root, "fake-claude")},
				"codex":    map[string]any{"executable": filepath.Join(root, "fake-codex"), "provider": map[string]any{"base_url": "http://127.0.0.1:1/v1"}},
				"writable": map[string][]string{"reviewer": {probe}},
			})
			v = mutate(t, bin, repo, home, v, "review", "review-"+c.backend, nil)
			v = waitReview(t, bin, repo, home)
			got := readProbe(t, filepath.Join(probe, c.backend+".json"))
			review := runOf(t, v)["review"].(map[string]any)
			conf := review["confinement"].(map[string]any)
			if strings.Contains(jsonText(conf["writable"]), filepath.Join(root, ".claude")) {
				t.Fatalf("the real Claude config directory is writable: %v", conf)
			}
			if c.backend == "claude" {
				want := map[string]string{"credentials": "denied", "state": "denied", "settings": "denied", "hooks": "denied", "codex-config": "denied"}
				for k, w := range want {
					if got[k] != w {
						t.Fatalf("claude reviewer access %s = %q, want %q (all: %v)", k, got[k], w, got)
					}
				}
				if conf["status"] != "degraded" || !strings.Contains(conf["reason"].(string), "Claude") {
					t.Fatalf("claude confinement record = %v", conf)
				}
				if screen := terminalOutput(t, bin, repo, home, "q\n"); !strings.Contains(screen, "Confinement: reviewer DEGRADED") {
					t.Fatalf("terminal must show the degraded Claude reviewer:\n%s", screen)
				}
				return
			}
			if got["codex-config"] != "denied" || conf["status"] != "degraded" || !strings.Contains(conf["reason"].(string), "Codex") || strings.Contains(jsonText(conf["writable"]), filepath.Join(root, ".codex")) {
				t.Fatalf("codex config must stay read-only and be recorded as degraded: probe %v record %v", got, conf)
			}
			if screen := terminalOutput(t, bin, repo, home, "q\n"); !strings.Contains(screen, "Confinement: reviewer DEGRADED") {
				t.Fatalf("terminal must show the degraded Codex reviewer:\n%s", screen)
			}
		})
	}
}

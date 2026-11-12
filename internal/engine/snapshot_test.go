package engine

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

type failingSnapshotWriter struct{}

func (failingSnapshotWriter) Write([]byte) (int, error) { return 0, os.ErrPermission }
func (failingSnapshotWriter) Close() error              { return nil }

func TestMaterializeStopsCatFileAfterBlobWriteFailure(t *testing.T) {
	t.Parallel()
	_, repo, _ := fixture(t)
	large := make([]byte, 8<<20)
	if err := os.WriteFile(filepath.Join(repo, "large.bin"), large, 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "large.bin")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "large blob")
	commit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	snapshot, err := snapshotOf(repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		result <- materializeWithOpen(repo, snapshot, filepath.Join(filepath.Dir(repo), "failed-snapshot"), func(string, int, os.FileMode) (io.WriteCloser, error) {
			return failingSnapshotWriter{}, nil
		})
	}()
	select {
	case err := <-result:
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("materialize error = %v, want write failure", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("materialize hung after a blob write failure")
	}
}

func TestReviewerReceivesReadOnlyFullTreeSnapshot(t *testing.T) {
	t.Parallel()
	bin, repo, home := fixture(t)
	commit := func(msg string) string {
		git(t, repo, "add", "-A")
		git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", msg)
		return strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	}
	os.MkdirAll(filepath.Join(repo, "lib"), 0700)
	os.WriteFile(filepath.Join(repo, "lib", "context.go"), []byte("package lib\n\nfunc Helper() int { return 7 }\n"), 0600)
	os.WriteFile(filepath.Join(repo, "run.sh"), []byte("#!/bin/sh\n"), 0700)
	target := commit("base")
	os.WriteFile(filepath.Join(repo, "sample.txt"), []byte("review me\n"), 0600)
	candidate := commit("candidate")
	os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("not committed\n"), 0600)
	v := axi(t, bin, repo, home, `{"version":1,"op":"attach"}`)
	installFake(t, home, `
cwd=os.getcwd()
seen={}
for dirpath,dirs,files in os.walk(cwd):
 for name in files:
  p=os.path.join(dirpath,name)
  seen[os.path.relpath(p,cwd)]=oct(os.stat(p).st_mode&0o777)
probe={'files':seen,'context':open(os.path.join(cwd,'lib','context.go')).read()}
try:
 open(os.path.join(cwd,'lib','context.go'),'a').write('tampered')
 probe['write']='allowed'
except OSError as e:
 probe['write']='denied'
try:
 open(os.path.join(cwd,'new.txt'),'w').write('x')
 probe['create']='allowed'
except OSError as e:
 probe['create']='denied'
json.dump(probe,open(root+'/snapshot-probe.json','w'))
`)
	q := map[string]any{"version": 1, "op": "prepare", "run_id": v["run_id"], "input_id": v["input_id"], "expected": v["token"], "gate_id": "none", "action_id": "prepare", "intent": "Review sample change", "target": target}
	b, _ := json.Marshal(q)
	v = mutate(t, bin, repo, home, axi(t, bin, repo, home, string(b)), "resolve-rules", "rules", nil)
	mutate(t, bin, repo, home, v, "review", "review", nil)
	done := waitReview(t, bin, repo, home)

	var probe struct {
		Files   map[string]string
		Context string
		Write   string
		Create  string
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(home), "snapshot-probe.json"))
	if err != nil {
		t.Fatalf("reviewer probe missing: %v; run %v", err, runOf(t, done))
	}
	json.Unmarshal(raw, &probe)
	want := map[string]string{"lib/context.go": "0o444", "run.sh": "0o555", "sample.txt": "0o444"}
	if len(probe.Files) != len(want) {
		t.Fatalf("snapshot files = %v, want exactly %v", probe.Files, want)
	}
	for name, mode := range want {
		if probe.Files[name] != mode {
			t.Fatalf("snapshot files = %v, want %v", probe.Files, want)
		}
	}
	if !strings.Contains(probe.Context, "Helper") || probe.Write != "denied" || probe.Create != "denied" {
		t.Fatalf("snapshot must hold unchanged files read-only: %+v", probe)
	}

	review := runOf(t, done)["review"].(map[string]any)
	snapshot, _ := review["snapshot"].(map[string]any)
	tree := strings.TrimSpace(git(t, repo, "rev-parse", candidate+"^{tree}"))
	if snapshot == nil || snapshot["tree"] != tree || snapshot["commit"] != candidate || snapshot["entries"] != float64(3) || snapshot["check"] != "unchanged" {
		t.Fatalf("review snapshot evidence = %v, want tree %s", snapshot, tree)
	}
	listing := git(t, repo, "ls-tree", "-r", "-z", "-l", "--full-tree", candidate)
	if snapshot["digest"] != runner.BytesDigest([]byte(listing)) {
		t.Fatalf("snapshot digest %v does not bind the candidate tree listing", snapshot["digest"])
	}
	captured, _ := os.ReadFile(filepath.Join(filepath.Dir(home), "captured.json"))
	var c struct {
		Input struct {
			Snapshot map[string]any
			Contract string
		} `json:"input"`
	}
	json.Unmarshal(captured, &c)
	if c.Input.Snapshot["Digest"] != snapshot["digest"] || c.Input.Snapshot["Tree"] != tree || strings.Contains(c.Input.Contract, "No repository checkout") || !strings.Contains(c.Input.Contract, "read-only snapshot") {
		t.Fatalf("reviewer input must bind the snapshot digest: %v / %s", c.Input.Snapshot, c.Input.Contract)
	}
}

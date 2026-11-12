package repository

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "HOME="+dir)
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git: %s %v", b, err)
	}
	return string(b)
}

type failingSnapshotWriter struct{}

func (failingSnapshotWriter) Write([]byte) (int, error) { return 0, os.ErrPermission }
func (failingSnapshotWriter) Close() error              { return nil }

func TestMaterializeStopsCatFileAfterBlobWriteFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-b", "main")
	large := make([]byte, 8<<20)
	if err := os.WriteFile(filepath.Join(repo, "large.bin"), large, 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "large.bin")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "large blob")
	commit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	snapshot, err := SnapshotOf(repo, commit)
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

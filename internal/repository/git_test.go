package repository

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuggestTargetPrefersConfiguredUpstream(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "base.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "base")
	git(t, repo, "branch", "upstream")
	git(t, repo, "checkout", "-b", "feature")
	git(t, repo, "branch", "--set-upstream-to=upstream")
	if got := SuggestTarget(repo); got != "upstream" {
		t.Fatalf("suggested target = %q, want upstream", got)
	}
}

func TestReadBoundedRetainsPrefix(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	content := strings.Repeat("bounded content\n", 100000)
	if err := os.WriteFile(filepath.Join(repo, "large.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "large.txt")
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "large")
	args := []string{"cat-file", "blob", "HEAD:large.txt"}
	b, err := ReadBounded(repo, 1000, args...)
	if !errors.Is(err, ErrReadLimit) || string(b) != content[:1000] {
		t.Fatalf("bounded read: bytes=%d error=%v", len(b), err)
	}
	b, err = Read(repo, args...)
	if !errors.Is(err, ErrReadLimit) || b != nil {
		t.Fatalf("default read accepted oversized output: bytes=%d error=%v", len(b), err)
	}
	b, err = ReadBounded(repo, len(content), args...)
	if err != nil || string(b) != content {
		t.Fatalf("exact limit: bytes=%d error=%v", len(b), err)
	}
}

func TestReadBoundedStopsProducer(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	marker := filepath.Join(repo, "drained")
	producer := `!exec python3 -c 'import sys; sys.stdout.buffer.write(b"x" * (8 * 1024 * 1024)); sys.stdout.flush(); open("drained", "w").close()'`
	data, err := ReadBounded(repo, 1024, "-c", "alias.produce="+producer, "produce")
	if !errors.Is(err, ErrReadLimit) || string(data) != strings.Repeat("x", 1024) {
		t.Fatalf("bounded producer: bytes=%d error=%v", len(data), err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("oversized git output was drained to completion: marker error=%v", err)
	}
}

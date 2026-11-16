package repository

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBoundedRetainsPrefix(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	content := strings.Repeat("bounded content\n", 100000)
	os.WriteFile(filepath.Join(repo, "large.txt"), []byte(content), 0600)
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

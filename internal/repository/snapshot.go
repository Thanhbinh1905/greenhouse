package repository

import (
	"bufio"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

type TreeSnapshot struct {
	Commit  string `json:"commit"`
	Tree    string `json:"tree"`
	Digest  string `json:"digest"`
	Entries int    `json:"entries"`
	Bytes   int64  `json:"bytes"`
	Check   string `json:"check,omitempty"`
}

type treeEntry struct {
	mode, kind, object, path string
	size                     int64
}

const (
	snapshotEntryLimit = 100000
	snapshotByteLimit  = 512 << 20
)

func TreeListing(worktree, commit string) ([]byte, []treeEntry, error) {
	c := exec.Command("git", "-C", worktree, "ls-tree", "-r", "-z", "-l", "--full-tree", commit)
	c.Env = gitEnv()
	out := runner.NewBuffer(64 << 20)
	c.Stdout = out
	if err := c.Run(); err != nil || out.Exceeded() {
		return nil, nil, errors.New("cannot list the candidate tree")
	}
	var entries []treeEntry
	for _, record := range strings.Split(out.String(), "\x00") {
		if record == "" {
			continue
		}
		meta, path, ok := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 4 || !fs.ValidPath(path) {
			return nil, nil, errors.New("unexpected candidate tree entry")
		}
		e := treeEntry{mode: fields[0], kind: fields[1], object: fields[2], path: path}
		if e.kind == "blob" {
			e.size, _ = strconv.ParseInt(fields[3], 10, 64)
		}
		entries = append(entries, e)
	}
	return out.Bytes(), entries, nil
}

func gitEnv() []string {
	var env []string
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			env = append(env, v)
		}
	}
	return append(env, "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1")
}

func SnapshotOf(worktree, commit string) (*TreeSnapshot, error) {
	listing, entries, err := TreeListing(worktree, commit)
	if err != nil {
		return nil, err
	}
	tree, err := Read(worktree, "rev-parse", "--verify", commit+"^{tree}")
	if err != nil {
		return nil, errors.New("cannot resolve the candidate tree")
	}
	s := &TreeSnapshot{Commit: commit, Tree: strings.TrimSpace(string(tree)), Digest: runner.BytesDigest(listing), Entries: len(entries)}
	for _, e := range entries {
		s.Bytes += e.size
	}
	if s.Entries > snapshotEntryLimit || s.Bytes > snapshotByteLimit {
		return nil, fmt.Errorf("candidate tree exceeds the review snapshot limit (%d entries, %d MiB)", snapshotEntryLimit, snapshotByteLimit>>20)
	}
	return s, nil
}

// materialize writes the exact snapshot listing to root from the object store,
// so attributes such as export-ignore or smudge filters cannot alter it.
func Materialize(worktree string, s *TreeSnapshot, root string) error {
	return materializeWithOpen(worktree, s, root, func(path string, flags int, mode os.FileMode) (io.WriteCloser, error) {
		return os.OpenFile(path, flags, mode)
	})
}

func materializeWithOpen(worktree string, s *TreeSnapshot, root string, openFile func(string, int, os.FileMode) (io.WriteCloser, error)) (retErr error) {
	listing, entries, err := TreeListing(worktree, s.Commit)
	if err != nil {
		return err
	}
	if runner.BytesDigest(listing) != s.Digest {
		return errors.New("candidate tree differs from the admitted snapshot")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return err
	}
	c := exec.Command("git", "-C", worktree, "cat-file", "--batch")
	c.Env = gitEnv()
	stdin, err := c.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return err
	}
	if err = c.Start(); err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			c.Process.Kill()
		}
		c.Wait()
	}()
	defer stdin.Close()
	reader := bufio.NewReader(stdout)
	var dirs []string
	for _, e := range entries {
		target := filepath.Join(root, filepath.FromSlash(e.path))
		parent := filepath.Dir(target)
		if err = os.MkdirAll(parent, 0700); err != nil {
			return err
		}
		if e.kind == "commit" {
			if err = os.Mkdir(target, 0700); err != nil {
				return err
			}
			dirs = append(dirs, target)
			continue
		}
		if e.kind != "blob" {
			return errors.New("unexpected candidate tree object")
		}
		if _, err = fmt.Fprintln(stdin, e.object); err != nil {
			return err
		}
		header, err := reader.ReadString('\n')
		fields := strings.Fields(header)
		if err != nil || len(fields) != 3 || fields[0] != e.object || fields[1] != "blob" {
			return errors.New("cannot read candidate blob")
		}
		size, _ := strconv.ParseInt(fields[2], 10, 64)
		if e.mode == "120000" {
			link := make([]byte, size)
			if _, err = io.ReadFull(reader, link); err != nil {
				return err
			}
			if err = os.Symlink(string(link), target); err != nil {
				return err
			}
		} else {
			perm := os.FileMode(0444)
			if e.mode == "100755" {
				perm = 0555
			}
			f, err := openFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			_, err = io.CopyN(f, reader, size)
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
			if err = os.Chmod(target, perm); err != nil {
				return err
			}
		}
		if _, err = reader.Discard(1); err != nil {
			return err
		}
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return err
	})
	if err != nil {
		return err
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if err = os.Chmod(dir, 0555); err != nil {
			return err
		}
	}
	return nil
}

func VerifySnapshot(worktree string, s *TreeSnapshot, root string) error {
	_, entries, err := TreeListing(worktree, s.Commit)
	if err != nil {
		return err
	}
	want := map[string]treeEntry{}
	for _, e := range entries {
		want[e.path] = e
	}
	seen := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		e, ok := want[rel]
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		if !ok {
			if d.IsDir() && info.Mode().Perm() == 0555 {
				return nil
			}
			return fmt.Errorf("unexpected path %s", rel)
		}
		seen++
		switch e.mode {
		case "160000":
			if !d.IsDir() {
				return fmt.Errorf("%s changed type", rel)
			}
			return filepath.SkipDir
		case "120000":
			link, err := os.Readlink(path)
			if err != nil || gitObject(e.object, []byte(link)) != e.object {
				return fmt.Errorf("%s changed", rel)
			}
			return nil
		}
		perm := os.FileMode(0444)
		if e.mode == "100755" {
			perm = 0555
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != perm {
			return fmt.Errorf("%s changed mode", rel)
		}
		content, err := os.ReadFile(path)
		if err != nil || gitObject(e.object, content) != e.object {
			return fmt.Errorf("%s changed", rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if seen != len(entries) {
		return errors.New("snapshot entries missing")
	}
	return nil
}

func gitObject(like string, content []byte) string {
	var h hash.Hash = sha1.New()
	if len(like) == 64 {
		h = sha256.New()
	}
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

func RemoveSnapshot(root string) {
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			os.Chmod(path, 0700)
		}
		return nil
	})
	os.RemoveAll(root)
}

package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func reviewDir(execution string) string {
	return filepath.Join(os.TempDir(), "greenhouse-review-"+execution)
}

func reverifySnapshot(x *Execution) string {
	dir := reviewDir(x.ID)
	defer repository.RemoveSnapshot(dir)
	if x.Snapshot == nil {
		return "unverified: the review has no bound snapshot"
	}
	listing, _, err := repository.TreeListing(x.Source.Worktree, x.Snapshot.Commit)
	if err != nil {
		return "unverified: the candidate tree is unavailable"
	}
	if runner.BytesDigest(listing) != x.Snapshot.Digest {
		return "failed: the candidate tree differs from the bound digest"
	}
	tree := filepath.Join(dir, "tree")
	info, err := os.Lstat(tree)
	if os.IsNotExist(err) {
		return "verified after restart: the candidate tree matches the bound digest; the review copy was already removed"
	}
	if err != nil || !info.IsDir() {
		return "failed: the review copy is unreadable"
	}
	if err := repository.VerifySnapshot(x.Source.Worktree, x.Snapshot, tree); err != nil {
		return "failed: the review copy was modified: " + err.Error()
	}
	return "verified after restart: the review copy matches the bound digest"
}

func recordReviewEvidence(x *Execution, check string, confinement *runner.Confinement) {
	if confinement != nil {
		x.Confinement = confinement
	}
	if x.Snapshot == nil || check == "" {
		return
	}
	s := *x.Snapshot
	s.Check = check
	x.Snapshot = &s
}

func snapshotLine(s *repository.TreeSnapshot) string {
	check := s.Check
	if check == "" {
		check = "not yet checked"
	}
	files := "files"
	if s.Entries == 1 {
		files = "file"
	}
	return fmt.Sprintf("Snapshot: %s | %d %s | tree %s\nSnapshot digest: %s", check, s.Entries, files, s.Tree, s.Digest)
}

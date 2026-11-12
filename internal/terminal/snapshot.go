package terminal

import (
	"fmt"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

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

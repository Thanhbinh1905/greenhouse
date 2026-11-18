package terminal

import (
	"fmt"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

const transcriptTailLines = 40

func transcriptPage(run string, t *engine.TranscriptView) string {
	var page strings.Builder
	fmt.Fprintf(&page, "Transcript log for run %s\nRedacted reviewer, fixer, and pipeline output.\nFile: %s | %d of %d bytes\n", run, t.Path, t.Bytes, t.Limit)
	if t.Bytes == 0 && len(t.Steps) == 0 {
		page.WriteString("No output yet.\n")
		return page.String()
	}
	lines := strings.Split(strings.TrimRight(t.Content, "\n"), "\n")
	if len(lines) > transcriptTailLines {
		fmt.Fprintf(&page, "Showing the last %d of %d lines; AXI op transcript returns all of it.\n", transcriptTailLines, len(lines))
		lines = lines[len(lines)-transcriptTailLines:]
	}
	page.WriteString(strings.Join(lines, "\n") + "\n")
	for _, step := range t.Steps {
		fmt.Fprintf(&page, "\n=== %s output (%d of %d bytes) ===\n", step.Step, step.Next-step.Offset, step.Bytes)
		lines := strings.Split(strings.TrimRight(step.Content, "\n"), "\n")
		if len(lines) > transcriptTailLines {
			lines = lines[len(lines)-transcriptTailLines:]
		}
		page.WriteString(strings.Join(lines, "\n") + "\n")
	}
	return page.String()
}

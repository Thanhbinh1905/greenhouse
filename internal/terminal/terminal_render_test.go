package terminal

import (
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

func captureTerminal(t *testing.T, draw func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = previous; reader.Close() }()
	draw()
	writer.Close()
	b, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestScreenWrapsAtWordBoundaries(t *testing.T) {
	text := strings.Repeat("word ", 15) + "configuration is untrusted"
	got := captureTerminal(t, func() { screen(text) })
	want := "\033[2J\033[H" + strings.Repeat("word ", 14) + "word\nconfiguration is untrusted\n"
	if got != want {
		t.Fatalf("screen split a word:\ngot  %q\nwant %q", got, want)
	}
}

func TestWrapWordsKeepsIndentAndLongValuesWhole(t *testing.T) {
	value := strings.Repeat("a", 128)
	for _, width := range []int{55, 79, 99} {
		got := wrapWords("  run "+value+" status", width)
		if want := "  run\n" + value + "\n  status"; got != want {
			t.Fatalf("width %d:\ngot  %q\nwant %q", width, got, want)
		}
	}
}

func TestDisplayIDFitsWithSeparator(t *testing.T) {
	id := strings.Repeat("a", 32) + strings.Repeat("b", 32)
	for _, width := range []int{55, 79, 99} {
		got := displayID(id, width)
		if len([]rune(got)) > width-1 || (width-1 >= len(id)) != (got == id) {
			t.Fatalf("width %d: displayed ID %q", width, got)
		}
	}
}

func TestDisplayedFindingIDSelectsOnlyUniqueFinding(t *testing.T) {
	id := strings.Repeat("a", 32) + strings.Repeat("b", 32)
	displayed := displayID(id, 55)
	ids, whole, err := engine.ParseSelection(displayed, []string{id})
	if err != nil || whole || len(ids) != 1 || ids[0] != id {
		t.Fatalf("displayed ID cannot select its finding: %v %t %v", ids, whole, err)
	}
	other := id[:30] + "cc" + id[32:]
	if _, _, err := engine.ParseSelection(displayed, []string{id, other}); err == nil {
		t.Fatal("ambiguous abbreviated ID selected a finding")
	}
	if _, _, err := engine.ParseSelection(id+","+displayed, []string{id}); err == nil {
		t.Fatal("duplicate full and abbreviated selection accepted")
	}
}

func TestSettingsPromptsWrapAndSanitize(t *testing.T) {
	text := "Selected rules (comma-separated; ? means optional; file:/absolute/path):\n> "
	got := captureTerminal(t, func() { terminalPrompt("%s", text) })
	if !strings.HasSuffix(got, "\n> ") {
		t.Fatalf("input prompt spacing lost: %q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if len([]rune(line)) > 80 {
			t.Fatalf("prompt exceeds terminal width: %q", line)
		}
	}
	got = captureTerminal(t, func() { terminalPrompt("%s", "model\x1b[31m\n> ") })
	if strings.Contains(got, "\x1b") {
		t.Fatal("terminal control escaped prompt sanitizer")
	}
}

func TestWrapWordsPreservesColumnSpacing(t *testing.T) {
	text := "  [i] Inspect  [s] Settings"
	if got := wrapWords(text, 55); got != text {
		t.Fatalf("column spacing changed: got %q want %q", got, text)
	}
}

func TestMenuEntriesWrapAtomicallyAtSupportedWidths(t *testing.T) {
	entries := []string{"[i] Inspect", "[c] Criteria/diff", "[l] Load rules", "[s] Settings"}
	for _, width := range []int{56, 80, 100} {
		got := wrapMenuEntries(entries, width)
		for _, line := range strings.Split(got, "\n") {
			if len([]rune(line)) > width {
				t.Errorf("width %d: menu line exceeds width: %q", width, line)
			}
		}
		for _, entry := range entries {
			if !slices.Contains(strings.Split(got, "\n"), entry) && !strings.Contains(got, entry) {
				t.Errorf("width %d: menu entry split: %q in %q", width, entry, got)
			}
		}
	}
}

func TestFinalReadyScreenKeepsActionableValuesWhole(t *testing.T) {
	remote := "https://github.com/Thanhbinh1905/greenhouse-live-validation-t59-20261109.git"
	ref := "refs/heads/greenhouse/8281036a00c4cf49a236983480517711"
	head := "2c6e7616ff282be4536a0c9f5c574c049acd2617"
	url := "https://github.com/Thanhbinh1905/greenhouse-live-validation-t59-20261109/pull/2"
	r := &engine.Run{Pipeline: &engine.Pipeline{Status: "ci-ready", Worktree: "/work", Ref: ref, Stages: []engine.StageResult{
		{Name: "push", Status: "passed", Head: head, Commands: []engine.CheckResult{
			{Args: []string{"/usr/bin/git", "ls-remote", remote, ref}, Status: "passed"},
			{Args: []string{"/usr/bin/git", "push", remote, head + ":" + ref}, Status: "passed"},
		}},
		{Name: "pr", Status: "passed", Head: head, URL: url},
	}}}
	var page strings.Builder
	renderPipeline(&page, r, false)
	var lines []string
	for _, line := range strings.Split(page.String(), "\n") {
		lines = append(lines, strings.Split(wrapWords(line, 55), "\n")...)
	}
	for _, value := range []string{remote, ref, strings.TrimPrefix(ref, "refs/heads/"), head + ":" + ref, url} {
		if !slices.ContainsFunc(lines, func(line string) bool { return strings.TrimSpace(line) == value }) {
			t.Errorf("value not shown whole on its own line: %s\n%s", value, strings.Join(lines, "\n"))
		}
	}
}

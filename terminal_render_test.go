package main

import (
	"io"
	"os"
	"strings"
	"testing"
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

func TestWrapWordsKeepsIndentAndFitsIdentifiers(t *testing.T) {
	for _, width := range []int{55, 79, 99} {
		text := "  " + strings.Repeat("a", 128) + " status"
		got := wrapWords(text, width)
		for _, line := range strings.Split(got, "\n") {
			if len([]rune(line)) > width || !strings.HasPrefix(line, "  ") {
				t.Fatalf("width %d: invalid indented line %q", width, line)
			}
		}
		if !strings.Contains(got, "…") || !strings.HasSuffix(got, "status") {
			t.Fatalf("identifier abbreviation or label lost: %q", got)
		}
	}
}

func TestDisplayedFindingIDSelectsOnlyUniqueFinding(t *testing.T) {
	id := strings.Repeat("a", 32) + strings.Repeat("b", 32)
	displayed := wrapWords(id, 55)
	ids, whole, err := parseSelection(displayed, []string{id})
	if err != nil || whole || len(ids) != 1 || ids[0] != id {
		t.Fatalf("displayed ID cannot select its finding: %v %t %v", ids, whole, err)
	}
	other := id[:30] + "cc" + id[32:]
	if _, _, err := parseSelection(displayed, []string{id, other}); err == nil {
		t.Fatal("ambiguous abbreviated ID selected a finding")
	}
	if _, _, err := parseSelection(id+","+displayed, []string{id}); err == nil {
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

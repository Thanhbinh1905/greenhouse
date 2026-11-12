package terminal

import (
	"strconv"
	"testing"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

func TestParseTerminalNumbersAndSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text string
		want int
		ok   bool
	}{{"7", 7, true}, {"1000", 1000, true}, {"", 0, false}, {" 7", 0, false}, {"7 ", 0, false}, {"7x", 0, false}, {"+7", 0, false}, {"-1", 0, false}, {"07", 0, false}, {"99999999999999999999", 0, false}} {
		got, err := engine.ParseCount(tc.text)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("parseCount(%q) = %d, %v", tc.text, got, err)
		}
	}
	for _, tc := range []struct {
		text string
		ok   bool
	}{{"ask one-pass 7", true}, {"auto full 6", true}, {"ask one-pass", false}, {"ask one-pass 7 extra", false}, {"ask one-pass 7x", false}, {" ask one-pass 7", false}, {"ask  one-pass 7", false}} {
		mode, preset, limit, err := engine.ParseDraft(tc.text)
		if (err == nil) != tc.ok || tc.ok && mode+" "+preset+" "+strconv.Itoa(limit) != tc.text {
			t.Errorf("parseDraft(%q) = %q %q %d, %v", tc.text, mode, preset, limit, err)
		}
	}
}

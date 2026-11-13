package tally

import "testing"

func TestCountEmpty(t *testing.T) {
	var tl Tally
	if got := tl.Count("a"); got != 0 {
		t.Fatalf("Count = %d, want 0", got)
	}
}

package tally

import "testing"

func TestAcceptZeroValueAdd(t *testing.T) {
	var tl Tally
	tl.Add("go")
	tl.Add("go")
	if got := tl.Count("go"); got != 2 {
		t.Fatalf("Count = %d, want 2", got)
	}
}

func TestRegressInitializedTally(t *testing.T) {
	tl := Tally{counts: map[string]int{"a": 1}}
	tl.Add("a")
	tl.Add("b")
	if tl.Count("a") != 2 || tl.Count("b") != 1 || tl.Count("c") != 0 {
		t.Fatalf("counts = %v", tl.counts)
	}
}

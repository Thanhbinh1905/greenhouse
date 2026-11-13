package page

import "testing"

func TestCount(t *testing.T) {
	if got := Count(5, 2); got != 3 {
		t.Fatalf("Count = %d, want 3", got)
	}
}

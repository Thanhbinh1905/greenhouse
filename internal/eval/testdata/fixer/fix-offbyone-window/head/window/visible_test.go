package window

import "testing"

func TestSizes(t *testing.T) {
	if got := Sizes(5, 2); got != 4 {
		t.Fatalf("Sizes(5, 2) = %d, want 4", got)
	}
}

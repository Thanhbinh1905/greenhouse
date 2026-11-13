package ratio

import "testing"

func TestRateHalf(t *testing.T) {
	if got := Rate(1, 2); got != 50 {
		t.Fatalf("Rate = %d, want 50", got)
	}
}

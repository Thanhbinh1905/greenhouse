package ratio

import "testing"

func TestAcceptRateWithoutRequests(t *testing.T) {
	if got := Rate(0, 0); got != 0 {
		t.Fatalf("Rate(0, 0) = %d, want 0", got)
	}
}

func TestRegressRate(t *testing.T) {
	if got := Rate(1, 4); got != 25 {
		t.Fatalf("Rate(1, 4) = %d, want 25", got)
	}
}

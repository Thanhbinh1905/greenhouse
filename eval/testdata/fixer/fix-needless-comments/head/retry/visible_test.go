package retry

import "testing"

func TestBackoffFirstAttempt(t *testing.T) {
	if got := Backoff(0); got != base {
		t.Fatalf("Backoff(0) = %v, want %v", got, base)
	}
}

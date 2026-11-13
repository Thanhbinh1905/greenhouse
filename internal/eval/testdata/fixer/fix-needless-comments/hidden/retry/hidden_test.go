package retry

import (
	"os"
	"strings"
	"testing"
	"time"
)

func source(t *testing.T) string {
	b, err := os.ReadFile("retry.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAcceptNarratingCommentsRemoved(t *testing.T) {
	src := source(t)
	for _, c := range []string{"start with the base delay", "double the delay for each attempt", "cap the delay at the limit"} {
		if strings.Contains(src, c) {
			t.Errorf("narrating comment %q remains", c)
		}
	}
}

func TestRegressBackoffAndRequiredComments(t *testing.T) {
	for attempt, want := range map[int]time.Duration{0: 100 * time.Millisecond, 3: 800 * time.Millisecond, 20: 20 * time.Second} {
		if got := Backoff(attempt); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
	src := source(t)
	for _, c := range []string{"// limit stays below the load balancer's 30s idle timeout.", "// Backoff returns the delay before the given zero-based retry attempt."} {
		if !strings.Contains(src, c) {
			t.Errorf("required comment %q removed", c)
		}
	}
}

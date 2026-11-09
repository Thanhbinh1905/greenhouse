package retry

import "time"

const (
	base = 100 * time.Millisecond
	// limit stays below the load balancer's 30s idle timeout.
	limit = 20 * time.Second
)

// Backoff returns the delay before the given zero-based retry attempt.
func Backoff(attempt int) time.Duration {
	d := base
	for i := 0; i < attempt && d < limit; i++ {
		d *= 2
	}
	if d > limit {
		d = limit
	}
	return d
}

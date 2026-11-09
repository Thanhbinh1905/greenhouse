package retry

import "time"

const (
	base = 100 * time.Millisecond
	// limit stays below the load balancer's 30s idle timeout.
	limit = 20 * time.Second
)

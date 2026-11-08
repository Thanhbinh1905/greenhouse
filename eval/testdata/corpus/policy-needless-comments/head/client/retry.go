package client

import "time"

// MaxAttempts is the number of attempts before a request fails.
const MaxAttempts = 5

// retryDelay returns the delay before the given zero-based retry attempt.
func retryDelay(attempt int) time.Duration {
	// set the base delay to 100 milliseconds
	delay := 100 * time.Millisecond
	// loop over the attempts and double the delay each time
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	// The payment gateway closes idle connections after 6 seconds
	// (https://example.com/gateway-limits), so the delay stays below that.
	if delay > 5*time.Second {
		delay = 5 * time.Second
	}
	// return the delay
	return delay
}

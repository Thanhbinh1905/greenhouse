package text

import (
	"strings"
	"sync"
)

// Words returns the number of whitespace-separated words in s.
func Words(s string) int {
	return len(strings.Fields(s))
}

// CountAll returns the total number of words across docs, counting each
// document in its own goroutine.
func CountAll(docs []string) int {
	total := 0
	var wg sync.WaitGroup
	for _, d := range docs {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			total += Words(d)
		}(d)
	}
	wg.Wait()
	return total
}

package text

import "strings"

// Words returns the number of whitespace-separated words in s.
func Words(s string) int {
	return len(strings.Fields(s))
}

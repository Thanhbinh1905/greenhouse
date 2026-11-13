package list

// First returns up to n leading items.
func First(items []string, n int) []string {
	if n < 0 {
		n = 0
	}
	if n > len(items) {
		n = len(items)
	}
	return items[:n]
}

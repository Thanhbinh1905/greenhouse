package list

// First returns up to n leading items.
func First(items []string, n int) []string {
	return items[:n]
}

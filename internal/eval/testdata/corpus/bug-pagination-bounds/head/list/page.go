package list

// Count returns the number of pages needed for n items of the given size.
func Count(n, size int) int {
	if size <= 0 {
		return 0
	}
	return (n + size - 1) / size
}

// Page returns the items on the zero-based page. The last page may hold
// fewer than size items.
func Page(items []string, page, size int) []string {
	start := page * size
	if start >= len(items) {
		return nil
	}
	end := start + size
	return items[start:end]
}

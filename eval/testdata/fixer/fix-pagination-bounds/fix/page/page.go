package page

// Count returns the number of pages of the given size needed for n items.
func Count(n, size int) int {
	if size <= 0 {
		return 0
	}
	return (n + size - 1) / size
}

// Page returns the items on the zero-based page of the given size.
func Page(items []string, page, size int) []string {
	start := page * size
	if start >= len(items) {
		return nil
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

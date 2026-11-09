package page

// Count returns the number of pages of the given size needed for n items.
func Count(n, size int) int {
	if size <= 0 {
		return 0
	}
	return (n + size - 1) / size
}

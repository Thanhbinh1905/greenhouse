package list

// Count returns the number of pages needed for n items of the given size.
func Count(n, size int) int {
	if size <= 0 {
		return 0
	}
	return (n + size - 1) / size
}

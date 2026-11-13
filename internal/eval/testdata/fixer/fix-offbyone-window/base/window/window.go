package window

// Sizes reports how many full windows of width k fit in n values.
func Sizes(n, k int) int {
	if k <= 0 || k > n {
		return 0
	}
	return n - k + 1
}

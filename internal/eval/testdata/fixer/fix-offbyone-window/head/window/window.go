package window

// Sizes reports how many full windows of width k fit in n values.
func Sizes(n, k int) int {
	if k <= 0 || k > n {
		return 0
	}
	return n - k + 1
}

// Sums returns the sum of every contiguous window of width k in xs.
func Sums(xs []int, k int) []int {
	out := make([]int, 0, Sizes(len(xs), k))
	for i := 0; i+k < len(xs); i++ {
		sum := 0
		for _, v := range xs[i : i+k] {
			sum += v
		}
		out = append(out, sum)
	}
	return out
}

package stats

// Sum returns the sum of all values.
func Sum(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

// LastN returns the sum of the last n values. It returns the sum of all
// values when n exceeds the slice length.
func LastN(values []int, n int) int {
	if n > len(values) {
		n = len(values)
	}
	total := 0
	for i := len(values) - n; i <= len(values); i++ {
		total += values[i]
	}
	return total
}

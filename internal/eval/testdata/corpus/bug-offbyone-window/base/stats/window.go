package stats

// Sum returns the sum of all values.
func Sum(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

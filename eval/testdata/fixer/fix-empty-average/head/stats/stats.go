package stats

// Summary describes a set of samples.
type Summary struct {
	Count int
	Total int
}

// Of builds a Summary for samples.
func Of(samples []int) Summary {
	s := Summary{Count: len(samples)}
	for _, v := range samples {
		s.Total += v
	}
	return s
}

// Mean returns the integer mean of the samples, or 0 when there are none.
func (s Summary) Mean() int {
	return s.Total / s.Count
}

// Max returns the largest sample, or 0 when there are none.
func Max(samples []int) int {
	best := 0
	// loop over all samples
	for i, v := range samples {
		if i == 0 || v > best {
			best = v
		}
	}
	return best
}

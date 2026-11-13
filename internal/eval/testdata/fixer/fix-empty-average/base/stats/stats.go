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

package ratio

// Stats holds request counters.
type Stats struct {
	Hits, Requests int
}

// Rate returns hits per request as a whole percentage, or 0 without requests.
func Rate(hits, requests int) int {
	if requests == 0 {
		return 0
	}
	return hits * 100 / requests
}

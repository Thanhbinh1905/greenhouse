package tally

// Tally counts words.
type Tally struct {
	counts map[string]int
}

// Count returns how many times word was added.
func (t *Tally) Count(word string) int {
	return t.counts[word]
}

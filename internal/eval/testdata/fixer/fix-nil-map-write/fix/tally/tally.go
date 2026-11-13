package tally

// Tally counts words. The zero value is ready to use.
type Tally struct {
	counts map[string]int
}

// Add records one occurrence of word.
func (t *Tally) Add(word string) {
	if t.counts == nil {
		t.counts = map[string]int{}
	}
	t.counts[word]++
}

// Count returns how many times word was added.
func (t *Tally) Count(word string) int {
	return t.counts[word]
}

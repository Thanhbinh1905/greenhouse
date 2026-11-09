package stats

import "testing"

func TestOf(t *testing.T) {
	if s := Of([]int{1, 2, 3}); s.Count != 3 || s.Total != 6 {
		t.Fatalf("Of = %+v", s)
	}
}

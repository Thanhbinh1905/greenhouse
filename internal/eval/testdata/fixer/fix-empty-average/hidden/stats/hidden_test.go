package stats

import "testing"

func TestAcceptMeanOfNoSamples(t *testing.T) {
	if got := Of(nil).Mean(); got != 0 {
		t.Fatalf("Of(nil).Mean() = %d, want 0", got)
	}
}

func TestRegressMeanAndMax(t *testing.T) {
	if got := Of([]int{2, 4, 7}).Mean(); got != 4 {
		t.Fatalf("Mean = %d, want 4", got)
	}
	if got := Max([]int{-3, -1, -2}); got != -1 {
		t.Fatalf("Max = %d, want -1", got)
	}
	if got := Max(nil); got != 0 {
		t.Fatalf("Max(nil) = %d, want 0", got)
	}
}

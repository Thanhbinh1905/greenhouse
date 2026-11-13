package window

import (
	"reflect"
	"testing"
)

func TestAcceptSumsIncludesLastWindow(t *testing.T) {
	if got := Sums([]int{1, 2, 3, 4}, 2); !reflect.DeepEqual(got, []int{3, 5, 7}) {
		t.Fatalf("Sums = %v, want [3 5 7]", got)
	}
	if got := Sums([]int{4, 5}, 2); !reflect.DeepEqual(got, []int{9}) {
		t.Fatalf("Sums of one window = %v, want [9]", got)
	}
}

func TestRegressSumsShortInput(t *testing.T) {
	if got := Sums([]int{1, 2}, 3); len(got) != 0 {
		t.Fatalf("Sums with k > len = %v, want empty", got)
	}
	if got := Sums([]int{1, 2, 3}, 1)[0]; got != 1 {
		t.Fatalf("first window = %d, want 1", got)
	}
}

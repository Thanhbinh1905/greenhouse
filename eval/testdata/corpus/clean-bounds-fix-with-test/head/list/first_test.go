package list

import (
	"reflect"
	"testing"
)

func TestFirstClampsN(t *testing.T) {
	items := []string{"a", "b"}
	for _, tc := range []struct {
		n    int
		want []string
	}{{-1, []string{}}, {0, []string{}}, {1, []string{"a"}}, {5, []string{"a", "b"}}} {
		if got := First(items, tc.n); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("First(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

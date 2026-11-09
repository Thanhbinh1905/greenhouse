package page

import (
	"reflect"
	"testing"
)

func TestAcceptPartialLastPage(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	if got := Page(items, 2, 2); !reflect.DeepEqual(got, []string{"e"}) {
		t.Fatalf("Page = %v, want [e]", got)
	}
}

func TestRegressFullAndMissingPages(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	if got := Page(items, 1, 2); !reflect.DeepEqual(got, []string{"c", "d"}) {
		t.Fatalf("Page = %v, want [c d]", got)
	}
	if got := Page(items, 2, 2); got != nil {
		t.Fatalf("Page past the end = %v, want nil", got)
	}
}

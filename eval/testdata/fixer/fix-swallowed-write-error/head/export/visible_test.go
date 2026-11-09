package export

import (
	"strings"
	"testing"
)

func TestWriteCSV(t *testing.T) {
	var b strings.Builder
	if err := WriteCSV(&b, []Record{{"a", 1}}); err != nil || b.String() != "a,1\n" {
		t.Fatalf("WriteCSV = %q, %v", b.String(), err)
	}
}

package export

import (
	"errors"
	"strings"
	"testing"
)

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestAcceptWriteCSVReturnsWriteError(t *testing.T) {
	want := errors.New("disk full")
	if err := WriteCSV(failingWriter{want}, []Record{{"a", 1}, {"b", 2}}); !errors.Is(err, want) {
		t.Fatalf("WriteCSV error = %v, want %v", err, want)
	}
}

func TestRegressWriteCSVOutput(t *testing.T) {
	var b strings.Builder
	if err := WriteCSV(&b, []Record{{"a", 1}, {"b", 22}}); err != nil || b.String() != "a,1\nb,22\n" {
		t.Fatalf("WriteCSV = %q, %v", b.String(), err)
	}
	if err := WriteCSV(failingWriter{errors.New("unused")}, nil); err != nil {
		t.Fatalf("WriteCSV without records = %v, want nil", err)
	}
}

package engine

import "testing"

func TestDiffPathSeparatesGitDelimiterFromFilename(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ header, path string }{
		{"a/has space.txt\t", "has space.txt"},
		{"b/trailing space \t", "trailing space "},
		{`"b/has\ttab.txt"`, "has\ttab.txt"},
		{`"b/has\ttab.txt"` + "\t", "has\ttab.txt"},
		{"b/plain.txt", "plain.txt"},
		{"/dev/null", "/dev/null"},
	} {
		if got := diffPath(test.header); got != test.path {
			t.Errorf("diffPath(%q)=%q, want %q", test.header, got, test.path)
		}
	}
}

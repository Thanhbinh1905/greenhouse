package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeModelCatalog(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pi := filepath.Join(root, "pi")
	if err := os.WriteFile(pi, []byte("#!/bin/sh\nprintf 'provider model\nnative chosen\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	catalog, err := nativeModelCatalog(pi)
	if _, ok := catalog["native/chosen"]; err != nil || !ok || len(catalog) != 1 {
		t.Fatalf("catalog: %v, %v", catalog, err)
	}
}

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeModelCatalog(t *testing.T) {
	root := t.TempDir()
	pi := filepath.Join(root, "pi")
	if err := os.WriteFile(pi, []byte("#!/bin/sh\nprintf 'provider model\nnative chosen\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	catalog, err := nativeModelCatalog(pi)
	if err != nil || !catalog["native/chosen"] || len(catalog) != 1 {
		t.Fatalf("catalog: %v, %v", catalog, err)
	}
}

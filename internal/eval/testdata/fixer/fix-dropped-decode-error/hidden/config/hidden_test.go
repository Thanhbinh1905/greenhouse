package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcceptInvalidJSONFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"addr":`), 0o600)
	if _, err := Load(p); err == nil {
		t.Fatal("Load accepted invalid JSON")
	}
}

func TestRegressLoad(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("Load accepted a missing file")
	}
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"workers":4}`), 0o600)
	if c, err := Load(p); err != nil || c.Workers != 4 {
		t.Fatalf("Load = %+v, %v", c, err)
	}
}

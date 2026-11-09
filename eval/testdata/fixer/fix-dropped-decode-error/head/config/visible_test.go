package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"addr":":80","workers":2}`), 0o600)
	if c, err := Load(p); err != nil || c.Addr != ":80" || c.Workers != 2 {
		t.Fatalf("Load = %+v, %v", c, err)
	}
}

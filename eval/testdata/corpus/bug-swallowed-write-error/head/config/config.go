package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Listen string `json:"listen"`
	Debug  bool   `json:"debug"`
}

// Load reads the configuration file at path.
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

// Save writes the configuration to path through a temporary file so readers
// never observe a partial file.
func Save(path string, c Config) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return nil
	}
	return os.Rename(tmp, path)
}

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

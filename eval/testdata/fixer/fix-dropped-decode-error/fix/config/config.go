package config

import (
	"encoding/json"
	"os"
)

// Config is the service configuration file.
type Config struct {
	Addr    string `json:"addr"`
	Workers int    `json:"workers"`
}

// Load reads the JSON configuration at path.
func Load(path string) (Config, error) {
	var cfg Config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

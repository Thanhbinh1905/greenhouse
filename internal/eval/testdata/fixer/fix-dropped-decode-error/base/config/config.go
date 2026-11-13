package config

// Config is the service configuration file.
type Config struct {
	Addr    string `json:"addr"`
	Workers int    `json:"workers"`
}

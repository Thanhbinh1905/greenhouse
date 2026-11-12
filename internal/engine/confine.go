package engine

import (
	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func confinementFor(role string, config ReviewerConfig, writable []string) (*runner.Policy, runner.Confinement) {
	paths := append([]string{}, writable...)
	if trusted, err := trustedBackends(); err == nil {
		paths = append(paths, trusted.Writable[role]...)
	}
	return runner.ConfinementFor(role, BackendOf(config), paths)
}

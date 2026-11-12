package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func confinementFor(role string, config ReviewerConfig, writable []string) (*runner.Policy, runner.Confinement) {
	paths := append([]string{}, writable...)
	if trusted, err := trustedBackends(); err == nil {
		paths = append(paths, trusted.Writable[role]...)
	}
	return runner.ConfinementFor(role, backendOf(config), paths)
}

func confinementLine(c *runner.Confinement) string {
	head := fmt.Sprintf("Confinement: %s %s enforced (ABI %d)", c.Role, c.Mechanism, c.ABI)
	if c.Status != "enforced" {
		head = fmt.Sprintf("Confinement: %s DEGRADED: %s", c.Role, c.Reason)
	}
	lines := []string{head}
	if len(c.Writable) == 0 {
		lines = append(lines, "  Writable: none")
	}
	for _, path := range c.Writable {
		lines = append(lines, "  Writable: "+path)
	}
	network := c.Network
	if len(c.Ports) > 0 {
		var ports []string
		for _, p := range c.Ports {
			ports = append(ports, strconv.Itoa(p))
		}
		network = "TCP connect only to port " + strings.Join(ports, ", ")
	}
	return strings.Join(append(lines, "  Network: "+network), "\n")
}

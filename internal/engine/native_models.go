package engine

import (
	"context"
	"errors"
	"strings"
	"time"
)

func nativeModelCatalog(executable string) (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, stderr := &boundedBuffer{limit: 65536}, &boundedBuffer{limit: 65536}
	_, err := runProcess(ctx, "", executable, []string{"--list-models", "--no-extensions", "--no-mcp", "--no-skills", "--no-context-files", "--no-approve", "--offline"}, nil, agentEnv(backendPi), out, stderr)
	if err != nil || out.exceeded {
		return nil, errors.New("pi model catalog unavailable; use pi --list-models")
	}
	catalog := map[string]bool{}
	for _, line := range strings.Split(out.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && !strings.EqualFold(fields[0], "provider") && validModelSelection(backendPi, fields[0]+"/"+fields[1]) {
			catalog[fields[0]+"/"+fields[1]] = true
		}
	}
	return catalog, nil
}

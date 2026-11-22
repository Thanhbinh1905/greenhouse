package engine

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func nativeModelCatalog(executable string) (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, stderr := runner.NewBuffer(65536), runner.NewBuffer(65536)
	_, err := runner.RunProcess(ctx, "", executable, []string{"--list-models", "--no-extensions", "--no-mcp", "--no-skills", "--no-context-files", "--no-approve", "--offline"}, nil, agentEnv(BackendPi), out, stderr)
	if err != nil || out.Exceeded() {
		return nil, errors.New("pi model catalog unavailable; use pi --list-models")
	}
	catalog := map[string]int{}
	for _, line := range strings.Split(out.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && !strings.EqualFold(fields[0], "provider") && validModelSelection(BackendPi, fields[0]+"/"+fields[1]) {
			catalog[fields[0]+"/"+fields[1]] = catalogTokens(append(fields, "")[2])
		}
	}
	return catalog, nil
}

func catalogTokens(size string) int {
	scale := 1.0
	switch {
	case strings.HasSuffix(size, "K"):
		scale, size = 1e3, strings.TrimSuffix(size, "K")
	case strings.HasSuffix(size, "M"):
		scale, size = 1e6, strings.TrimSuffix(size, "M")
	}
	n, err := strconv.ParseFloat(size, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return int(n * scale)
}

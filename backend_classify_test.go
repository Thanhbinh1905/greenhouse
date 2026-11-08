package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func recorded(t *testing.T, backend, name string) ([]byte, []byte, int) {
	t.Helper()
	base := filepath.Join("testdata", "backends", backend, name)
	stdout, err := os.ReadFile(base + ".stdout")
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(base + ".stderr")
	if err != nil {
		t.Fatal(err)
	}
	exit, err := os.ReadFile(base + ".exit")
	if err != nil {
		t.Fatal(err)
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(exit)))
	if err != nil {
		t.Fatal(err)
	}
	return stdout, stderr, code
}

func TestClassifyRecordedBackendOutputs(t *testing.T) {
	bangkok := time.FixedZone("+07", 7*3600)
	recordedAt := time.Date(2026, 11, 8, 20, 47, 17, 0, bangkok)
	for _, c := range []struct {
		backend, name, reason, reset string
	}{
		{"claude", "success", "", ""},
		{"claude", "auth", "auth", ""},
		{"claude", "quota", "quota", "2026-11-08T14:50:46Z"},
		{"claude", "api-rate-limit", "quota", ""},
		{"claude", "unavailable", "unavailable", ""},
		{"claude", "connection-refused", "unavailable", ""},
		{"codex", "success", "", ""},
		{"codex", "auth", "auth", ""},
		{"codex", "quota", "quota", "2026-11-08T14:47:00Z"},
		{"codex", "quota-date", "quota", "2026-11-11T13:47:00Z"},
		{"codex", "insufficient-quota", "quota", ""},
		{"codex", "rate-limit", "quota", ""},
		{"codex", "unavailable", "unavailable", ""},
		{"codex", "waiting-for-network", "unavailable", ""},
		{"pi", "auth", "auth", ""},
		{"pi", "quota", "quota", ""},
		{"pi", "rate-limit", "quota", ""},
		{"pi", "unavailable", "unavailable", ""},
		{"pi", "connection-refused", "unavailable", ""},
	} {
		stdout, stderr, code := recorded(t, c.backend, c.name)
		failure := classifyBackend(c.backend, code, stdout, stderr, recordedAt)
		if c.reason == "" {
			if failure != nil {
				t.Errorf("%s/%s: unexpected failure %+v", c.backend, c.name, failure)
			}
			continue
		}
		if failure == nil || failure.Reason != c.reason || failure.ResetAt != c.reset {
			t.Errorf("%s/%s: got %+v, want reason %s reset %q", c.backend, c.name, failure, c.reason, c.reset)
			continue
		}
		if failure.Message == "" || strings.Contains(failure.Message, "sk-") || len(failure.Message) > 300 {
			t.Errorf("%s/%s: unsafe or empty message %q", c.backend, c.name, failure.Message)
		}
	}
}

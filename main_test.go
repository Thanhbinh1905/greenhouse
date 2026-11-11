package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIHelp(t *testing.T) {
	bin, repo, home := fixture(t)
	for _, args := range [][]string{{"--help"}, {"-h"}, {"axi", "--help"}, {"axi", "-h"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+filepath.Join(home, "unused"), "GORACE=atexit_sleep_ms=0")
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("help must exit zero without reading stdin: %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("help wrote stderr: %q", stderr.String())
			}
			for _, want := range []string{"Usage:", "interactive", "line", "axi", "stdin", "intent", "target", "merge-base", "grant", "review", "docs/embedding.md"} {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("help missing %q: %s", want, stdout.String())
				}
			}
			if _, err := os.Stat(home); !os.IsNotExist(err) {
				t.Fatalf("help must not initialize state: %v", err)
			}
		})
	}
}

func TestCLIAXIWithoutHelp(t *testing.T) {
	bin, repo, home := fixture(t)
	request := `{"version":2,"op":"attach","host":{"name":"example","id":"task-1"}}`
	first := axi(t, bin, repo, home, request)
	second := axi(t, bin, repo, home, request)
	if first["outcome"] != "ok" || first["run_id"] == nil || first["run_id"] != second["run_id"] || first["token"] != second["token"] {
		t.Fatalf("AXI attach did not round-trip: first=%v second=%v", first, second)
	}
	for _, input := range []struct{ request, message string }{{"", "EOF"}, {`{"help":true}`, `json: unknown field "help"`}} {
		cmd := exec.Command(bin, "axi")
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "GORACE=atexit_sleep_ms=0")
		cmd.Stdin = strings.NewReader(input.request)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			t.Fatalf("invalid AXI request must exit 1: %v", err)
		}
		want, err := json.Marshal(Response{Version: 1, Outcome: "error", Error: input.message})
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, '\n')
		if !bytes.Equal(stdout.Bytes(), want) || stderr.Len() != 0 {
			t.Fatalf("AXI error bytes changed: stdout=%q stderr=%q want=%q", stdout.Bytes(), stderr.Bytes(), want)
		}
	}
}

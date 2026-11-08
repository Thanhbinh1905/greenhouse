package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// shimEnv names the variable that switches this binary into shim mode. The
// daemon passes it through because Greenhouse only strips PI_, GIT_, POSSE_
// and HERDR_ variables from the reviewer environment.
const shimEnv = "GREENHOUSE_EVAL_SHIM"

type shimConfig struct {
	Pi       string `json:"pi"`
	Capture  string `json:"capture"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url,omitempty"`
}

type shimTiming struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Millis   int64     `json:"millis"`
	ExitCode int       `json:"exit_code"`
}

// runShim stands in for Pi as the configured reviewer executable. It records
// the exact argv and stdin Greenhouse delivers, optionally routes the provider
// to a base URL inside Greenhouse's private agent directory, runs the real Pi
// with the same arguments and input, and records its stdout.
func runShim(configPath string) int {
	b, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	var cfg shimConfig
	if err := strictDecode(b, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	code, err := shim(cfg, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	return code
}

func shim(cfg shimConfig, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	input, err := io.ReadAll(stdin)
	if err != nil {
		return 2, err
	}
	argv, _ := json.Marshal(args)
	if err := os.WriteFile(filepath.Join(cfg.Capture, "argv.json"), append(argv, '\n'), 0o600); err != nil {
		return 2, err
	}
	if err := os.WriteFile(filepath.Join(cfg.Capture, "input.json"), input, 0o600); err != nil {
		return 2, err
	}
	var env []string
	agentDir := ""
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, shimEnv+"=") {
			continue
		}
		if dir, ok := strings.CutPrefix(v, "PI_CODING_AGENT_DIR="); ok {
			agentDir = dir
		}
		env = append(env, v)
	}
	if cfg.BaseURL != "" {
		if agentDir == "" {
			return 2, errors.New("PI_CODING_AGENT_DIR unset; cannot route the provider")
		}
		models, _ := json.Marshal(map[string]any{"providers": map[string]any{cfg.Provider: map[string]string{"baseUrl": cfg.BaseURL}}})
		if err := os.MkdirAll(agentDir, 0o700); err != nil {
			return 2, err
		}
		if err := os.WriteFile(filepath.Join(agentDir, "models.json"), models, 0o600); err != nil {
			return 2, err
		}
	}
	events, err := os.OpenFile(filepath.Join(cfg.Capture, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 2, err
	}
	defer events.Close()
	diag, err := os.OpenFile(filepath.Join(cfg.Capture, "stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 2, err
	}
	defer diag.Close()
	cmd := exec.Command(cfg.Pi, args...)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = io.MultiWriter(stdout, events)
	cmd.Stderr = io.MultiWriter(stderr, diag)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	timing := shimTiming{Start: time.Now()}
	err = cmd.Run()
	timing.End = time.Now()
	timing.Millis = timing.End.Sub(timing.Start).Milliseconds()
	timing.ExitCode = -1
	if cmd.ProcessState != nil {
		timing.ExitCode = cmd.ProcessState.ExitCode()
	}
	t, _ := json.Marshal(timing)
	os.WriteFile(filepath.Join(cfg.Capture, "timing.json"), append(t, '\n'), 0o600)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return timing.ExitCode, nil
	}
	if err != nil {
		return 2, err
	}
	return 0, nil
}

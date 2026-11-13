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
	Pi      string `json:"pi"`
	Capture string `json:"capture"`
}

type shimTiming struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Millis   int64     `json:"millis"`
	ExitCode int       `json:"exit_code"`
}

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
	if len(args) == 1 && args[0] == "--list-models" {
		command := exec.Command(cfg.Pi, args...)
		command.Env = os.Environ()
		command.Stdout, command.Stderr = stdout, stderr
		if err := command.Run(); err != nil {
			return 1, err
		}
		return 0, nil
	}
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
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, shimEnv+"=") {
			continue
		}
		env = append(env, v)
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

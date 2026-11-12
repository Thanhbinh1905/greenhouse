package repository

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

func Read(path string, args ...string) ([]byte, error) {
	c := exec.Command("git", append([]string{"-C", path}, args...)...)
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			c.Env = append(c.Env, v)
		}
	}
	c.Env = append(c.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1")
	b := runner.NewBuffer(1024 * 1024)
	c.Stdout = b
	err := c.Run()
	if b.Exceeded() {
		return nil, errors.New("committed input exceeds 1 MiB")
	}
	return b.Bytes(), err
}
func CommittedTarget(path, target string) (string, error) {
	if target == "" || strings.HasPrefix(target, "-") || strings.ContainsAny(target, "\x00\r\n") {
		return "", errors.New("select a target Git branch or commit")
	}
	b, e := Read(path, "rev-parse", "--verify", target+"^{commit}")
	if e != nil || strings.TrimSpace(string(b)) == "" {
		return "", errors.New("target commit unavailable")
	}
	return strings.TrimSpace(string(b)), nil
}
func ReviewDiffBase(path, target, candidate string) (string, string, error) {
	base, err := Read(path, "merge-base", target, candidate)
	if err == nil {
		return strings.TrimSpace(string(base)), "", nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return target, "target and candidate have no merge-base; using requested target for review diff", nil
	}
	return "", "", fmt.Errorf("cannot compute review merge-base: %w", err)
}

func SuggestTarget(path string) string {
	for _, name := range []string{"refs/remotes/origin/HEAD", "main", "master", "HEAD^"} {
		if _, err := CommittedTarget(path, name); err == nil {
			return name
		}
	}
	return ""
}

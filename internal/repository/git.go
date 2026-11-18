package repository

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func Read(path string, args ...string) ([]byte, error) {
	b, err := ReadBounded(path, 1024*1024, args...)
	if err != nil {
		return nil, err
	}
	return b, nil
}

var ErrReadLimit = errors.New("committed input exceeds read limit")

type cappedOutput struct {
	data     bytes.Buffer
	limit    int
	exceeded bool
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if n > b.limit-b.data.Len() {
		b.exceeded = true
		p = p[:b.limit-b.data.Len()]
	}
	b.data.Write(p)
	if b.exceeded {
		return len(p), ErrReadLimit
	}
	return n, nil
}

func ReadBounded(path string, limit int, args ...string) ([]byte, error) {
	c := exec.Command("git", append([]string{"-C", path}, args...)...)
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			c.Env = append(c.Env, v)
		}
	}
	c.Env = append(c.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1")
	b := &cappedOutput{limit: limit}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = c.Start(); err != nil {
		return nil, err
	}
	_, readErr := io.Copy(b, stdout)
	if readErr != nil {
		stdout.Close()
		c.Process.Kill()
	}
	err = c.Wait()
	if readErr != nil && !b.exceeded {
		return b.data.Bytes(), readErr
	}
	if b.exceeded {
		return b.data.Bytes(), ErrReadLimit
	}
	return b.data.Bytes(), err
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

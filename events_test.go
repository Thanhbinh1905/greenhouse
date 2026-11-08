package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func watch(t *testing.T, bin, repo, home string, stream any, after any, waitMS int) map[string]any {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"version": 2, "op": "watch", "stream": stream, "after": after, "wait_ms": waitMS})
	return axi(t, bin, repo, home, string(b))
}

func TestEventSubscriptionRecoversThroughSnapshots(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, "import time; time.sleep(1.5)")

	first := watch(t, bin, repo, home, "", 0, 0)
	if first["outcome"] != "ok" || first["wait"] != "resync" || first["stream"] == "" || runOf(t, first)["token"] != v["token"] {
		t.Fatalf("a new subscriber must start from the authoritative snapshot: %v", first)
	}
	stream, after := first["stream"], first["sequence"]

	idle := watch(t, bin, repo, home, stream, after, 200)
	if idle["outcome"] != "ok" || idle["wait"] != "elapsed" || idle["events"] != nil || idle["error"] != nil || runOf(t, idle)["token"] != v["token"] {
		t.Fatalf("an elapsed wait is a successful empty answer: %v", idle)
	}

	waited := make(chan map[string]any, 1)
	go func() { waited <- watch(t, bin, repo, home, stream, after, 20000) }()
	time.Sleep(300 * time.Millisecond)
	review, _ := json.Marshal(map[string]any{"version": 2, "op": "review", "run_id": v["run_id"], "expected": v["token"], "input_id": v["input_id"], "gate_id": v["gate_id"], "action_id": "review-events"})
	started := axi(t, bin, repo, home, string(review))
	if started["outcome"] != "ok" || started["stream"] != stream || started["sequence"] != after {
		t.Fatalf("a response must name the last event committed before it: %v", started)
	}
	if replay := axi(t, bin, repo, home, string(review)); fmt.Sprint(replay) != fmt.Sprint(started) {
		t.Fatalf("a replay must return the recorded result unchanged:\n%v\n%v", replay, started)
	}
	notified := <-waited
	events, _ := notified["events"].([]any)
	if notified["wait"] != "events" || len(events) == 0 || notified["sequence"].(float64) <= after.(float64) {
		t.Fatalf("a mutation must wake the subscriber with sequenced events: %v", notified)
	}
	last := events[len(events)-1].(map[string]any)
	if last["run_id"] != v["run_id"] || last["status"] != "running" || last["sequence"] != notified["sequence"] {
		t.Fatalf("event does not describe the run change: %v", last)
	}
	after = notified["sequence"]

	during := watch(t, bin, repo, home, stream, after, 300)
	if during["wait"] != "elapsed" || runOf(t, during)["status"] != "running" {
		t.Fatalf("elapsed wait during a review: %v", during)
	}
	settled := map[string]any{}
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		settled = watch(t, bin, repo, home, stream, after, 5000)
		after = settled["sequence"]
		if runOf(t, settled)["status"] != "running" {
			break
		}
	}
	if settled["wait"] != "events" || runOf(t, settled)["status"] != "outstanding-findings" || runOf(t, settled)["review_used"] != float64(1) {
		t.Fatalf("an elapsed wait must not cancel or fail the review: %v", settled)
	}
	for _, e := range runOf(t, settled)["events"].([]any) {
		if kind := e.(map[string]any)["kind"]; kind == "cancel" || kind == "interrupted" {
			t.Fatalf("waiting recorded %v", kind)
		}
	}

	for _, gap := range []struct {
		stream, after any
	}{{stream, after.(float64) + 100}, {"unknown-stream", after}} {
		resync := watch(t, bin, repo, home, gap.stream, gap.after, 1000)
		if resync["wait"] != "resync" || runOf(t, resync)["token"] != runOf(t, settled)["token"] {
			t.Fatalf("gap %v must resync from the snapshot: %v", gap, resync)
		}
	}

	stopDaemon(t, home)
	restarted := watch(t, bin, repo, home, stream, after, 1000)
	if restarted["wait"] != "resync" || restarted["stream"] == stream || runOf(t, restarted)["status"] != "outstanding-findings" {
		t.Fatalf("a reconnect after restart must resync on a new stream: %v", restarted)
	}
	if fmt.Sprint(runOf(t, restarted)["findings"]) != fmt.Sprint(runOf(t, settled)["findings"]) {
		t.Fatalf("restart snapshot lost findings: %v", restarted)
	}
}

type terminalClient struct {
	stdin  io.WriteCloser
	output *bytes.Buffer
	lock   *sync.Mutex
	done   chan error
}

func startTerminal(t *testing.T, bin, repo, home string, env ...string) *terminalClient {
	t.Helper()
	c := exec.Command(bin)
	c.Dir = repo
	c.Env = append(os.Environ(), "GREENHOUSE_HOME="+home, "HOME="+filepath.Dir(home), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GREENHOUSE_TEST_PROVIDER_KEY=fixture-secret", "GORACE="+os.Getenv("GORACE")+" atexit_sleep_ms=0")
	c.Env = append(c.Env, env...)
	stdin, err := c.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	term := &terminalClient{stdin: stdin, output: &bytes.Buffer{}, lock: &sync.Mutex{}, done: make(chan error, 1)}
	c.Stdout = lockedWriter{term}
	c.Stderr = lockedWriter{term}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { term.done <- c.Wait() }()
	t.Cleanup(func() { stdin.Close(); c.Process.Kill() })
	return term
}

type lockedWriter struct{ term *terminalClient }

func (w lockedWriter) Write(b []byte) (int, error) {
	w.term.lock.Lock()
	defer w.term.lock.Unlock()
	return w.term.output.Write(b)
}

func (term *terminalClient) text() string {
	term.lock.Lock()
	defer term.lock.Unlock()
	return term.output.String()
}

func (term *terminalClient) waitFor(t *testing.T, want string) string {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if text := term.text(); strings.Contains(text, want) {
			return text
		}
	}
	t.Fatalf("terminal never showed %q:\n%s", want, term.text())
	return ""
}

func TestTerminalFollowsEventsWithoutInput(t *testing.T) {
	t.Parallel()
	bin, repo, home, v := prepareFixture(t)
	installFake(t, home, "")
	term := startTerminal(t, bin, repo, home)
	term.waitFor(t, "Events: live")
	before := strings.Count(term.text(), "Run "+v["run_id"].(string))
	if started := mutate(t, bin, repo, home, v, "review", "review-tui", nil); started["outcome"] != "ok" {
		t.Fatal(started)
	}
	screen := term.waitFor(t, "Stage: independent-review | outstanding-findings")
	if strings.Count(screen, "Run "+v["run_id"].(string)) <= before || !strings.Contains(screen, "Changed prose is redundant") {
		t.Fatalf("terminal did not redraw from events:\n%s", screen)
	}
	io.WriteString(term.stdin, "q\n")
	if err := <-term.done; err != nil {
		t.Fatalf("detach: %v\n%s", err, term.text())
	}
	if after := axi(t, bin, repo, home, `{"version":1,"op":"inspect"}`); runOf(t, after)["review_used"] != float64(1) {
		t.Fatalf("terminal changed the run: %v", after)
	}
}

func TestTerminalIgnoresOlderSnapshots(t *testing.T) {
	current := Response{Stream: "s1", Sequence: 7}
	for _, c := range []struct {
		update Response
		want   bool
	}{
		{Response{Stream: "s1", Sequence: 7}, false},
		{Response{Stream: "s1", Sequence: 6}, false},
		{Response{Stream: "s1", Sequence: 8}, true},
		{Response{Stream: "s2", Sequence: 1}, true},
	} {
		if got := newer(c.update, current); got != c.want {
			t.Fatalf("newer(%+v) = %v, want %v", c.update, got, c.want)
		}
	}
}

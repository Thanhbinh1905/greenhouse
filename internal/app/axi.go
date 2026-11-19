package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

func axiCommand(home, path string, args []string) (engine.Response, error) {
	var zero engine.Response
	op := args[0]
	switch op {
	case "inspect", "watch", "pipeline", "decide", "cancel", "transcript", "logs":
	default:
		return zero, fmt.Errorf("unsupported AXI command: %s", op)
	}
	q := engine.Request{Version: engine.MaxVersion, Op: op, Path: path, Client: "axi"}
	flags := flag.NewFlagSet(op, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&q.RunID, "run", "", "run identity")
	flags.StringVar(&q.RunID, "run_id", "", "run identity")
	flags.StringVar(&q.Input, "input_id", "", "input identity")
	flags.StringVar(&q.Gate, "gate_id", "", "gate identity")
	flags.StringVar(&q.Expected, "expected", "", "answer token")
	flags.StringVar(&q.Action, "action_id", "", "idempotency identity")
	flags.StringVar(&q.Intent, "intent", "", "run intent")
	flags.StringVar(&q.Target, "target", "", "target commit or branch")
	flags.StringVar(&q.Title, "title", "", "conventional-commit PR title")
	flags.StringVar(&q.Reason, "reason", "", "decision reason")
	flags.StringVar(&q.Choice, "choice", "", "decision choice")
	flags.StringVar(&q.Step, "step", "", "output stage")
	flags.Func("offset", "output byte offset", func(value string) error {
		offset, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			q.Offset = &offset
		}
		return err
	})
	flags.BoolVar(&q.Full, "full", false, "read full bounded log")
	flags.BoolVar(&q.Follow, "follow", false, "wait for new output")
	flags.StringVar(&q.Stream, "stream", "", "event stream")
	flags.Uint64Var(&q.After, "after", 0, "event sequence")
	flags.IntVar(&q.WaitMS, "wait_ms", 0, "wait in milliseconds")
	flags.IntVar(&q.Limit, "limit", 0, "decision budget")
	flags.Func("selected", "finding identity (repeatable)", func(value string) error { q.Selected = append(q.Selected, value); return nil })
	wait := flags.Duration("wait", 0, "bounded wait duration")
	if op == "decide" && len(args) > 1 && len(args[1]) > 0 && args[1][0] != '-' {
		q.Choice = args[1]
		args = append([]string{op}, args[2:]...)
	}
	if err := flags.Parse(args[1:]); err != nil {
		return zero, err
	}
	if flags.NArg() != 0 {
		return zero, errors.New("unexpected positional arguments")
	}
	if *wait < 0 || *wait > 24*time.Hour || q.WaitMS < 0 {
		return zero, errors.New("wait must be between zero and 24h")
	}
	if *wait > 0 {
		if q.WaitMS != 0 {
			return zero, errors.New("use either wait or wait_ms")
		}
		q.WaitMS = int(wait.Milliseconds())
	}
	mutation := op == "pipeline" || op == "decide" || op == "cancel"
	if mutation {
		state, err := engine.Send(home, engine.Request{Version: q.Version, Op: "inspect", Path: path, RunID: q.RunID}, true)
		if err != nil || state.Outcome != "ok" {
			return state, err
		}
		if state.Run == nil && op == "pipeline" {
			state, err = engine.Send(home, engine.Request{Version: q.Version, Op: "attach", Path: path}, true)
			if err != nil || state.Outcome != "ok" {
				return state, err
			}
		}
		if q.RunID == "" {
			q.RunID = state.RunID
		}
		if q.Input == "" {
			q.Input = state.Input
		}
		if q.Gate == "" {
			q.Gate = state.Gate
		}
		if q.Expected == "" {
			q.Expected = state.Token
		}
		if q.Action == "" {
			q.Action = fmt.Sprintf("cli-%d", time.Now().UnixNano())
		}
	}
	if op == "watch" && q.Stream == "" {
		state, err := sendAXI(home, engine.Request{Version: q.Version, Op: "watch", Path: path, RunID: q.RunID})
		if err != nil || state.Outcome != "ok" || q.WaitMS == 0 {
			return state, err
		}
		q.Stream, q.After = state.Stream, state.Sequence
	}
	return sendAXI(home, q)
}

func compactResponse(v engine.Response) map[string]any {
	b, _ := json.Marshal(v)
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		return map[string]any{"outcome": "error", "error": err.Error()}
	}
	for _, key := range []string{"rules", "rule_scope", "defaults", "defaults_sources", "config", "migrations", "models"} {
		delete(result, key)
	}
	if run, ok := result["run"].(map[string]any); ok {
		for _, key := range []string{"selected_diff", "reviewer", "review", "repair", "rule_snapshots", "configs", "events", "prior_repairs", "host_responses", "host_findings"} {
			delete(run, key)
		}
		if p, ok := run["pipeline"].(map[string]any); ok {
			for _, key := range []string{"reviews", "repairs", "command_digests", "plan", "grants"} {
				delete(p, key)
			}
		}
	}
	return result
}

func sendAXI(home string, q engine.Request) (engine.Response, error) {
	deadline := time.Now().Add(time.Duration(q.WaitMS) * time.Millisecond)
	for {
		response, err := engine.Send(home, q, true)
		if err != nil || q.Op != "watch" || response.Outcome != "refused" || response.Error != "storage commit in progress; retry the read request" || !time.Now().Before(deadline) {
			return response, err
		}
		time.Sleep(min(20*time.Millisecond, time.Until(deadline)))
		if q.WaitMS > 0 {
			q.WaitMS = max(0, int(time.Until(deadline).Milliseconds()))
		}
	}
}

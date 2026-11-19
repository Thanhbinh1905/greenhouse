package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

const defaultReviewBudget = 3 * time.Minute

func reviewBudget() (time.Duration, error) {
	cfg, err := trustedBackends()
	if err != nil {
		return 0, err
	}
	if cfg.ReviewBudget != "" {
		return time.ParseDuration(cfg.ReviewBudget)
	}
	return defaultReviewBudget, nil
}

type reviewSession struct {
	args    []string
	env     []string
	state   string
	temp    string
	id      string
	backend string
}

func removeArgs(args []string, flags ...string) []string {
	result := []string{}
	for i := 0; i < len(args); i++ {
		drop := false
		for _, flag := range flags {
			if args[i] == flag {
				drop = true
				break
			}
		}
		if !drop {
			result = append(result, args[i])
		}
	}
	return result
}

func newReviewSession(x *Execution, dir string) (reviewSession, error) {
	s := reviewSession{args: append([]string(nil), x.Args...), env: agentEnv(x.Config.Backend), state: filepath.Join(dir, "session"), temp: filepath.Join(dir, "tmp"), backend: BackendOf(x.Config)}
	if err := os.Mkdir(s.state, 0700); err != nil {
		return s, err
	}
	if err := os.Mkdir(s.temp, 0700); err != nil {
		return s, err
	}
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		env := []string{}
		for _, v := range s.env {
			if !strings.HasPrefix(v, key+"=") {
				env = append(env, v)
			}
		}
		s.env = append(env, key+"="+s.temp)
	}
	switch s.backend {
	case BackendPi:
		s.id = filepath.Join(s.state, "review.jsonl")
		s.args = append(removeArgs(s.args, "--no-session"), "--session", s.id)
		s.env = append(s.env, "PI_CODING_AGENT_SESSION_DIR="+s.state)
	case BackendClaude, BackendCodex:
		key, defaultDir := "CLAUDE_CONFIG_DIR", ".claude"
		copied := map[string]bool{".credentials.json": true, "settings.json": true, "settings.local.json": true}
		if s.backend == BackendCodex {
			key, defaultDir = "CODEX_HOME", ".codex"
			copied = map[string]bool{"auth.json": true, "config.toml": true, "models_cache.json": true}
		}
		native := os.Getenv(key)
		if native == "" {
			h, err := os.UserHomeDir()
			if err != nil {
				return s, err
			}
			native = filepath.Join(h, defaultDir)
		}
		entries, err := os.ReadDir(native)
		if err != nil && !os.IsNotExist(err) {
			return s, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if !copied[name] {
				continue
			}
			data, err := os.ReadFile(filepath.Join(native, name))
			if err != nil {
				return s, err
			}
			if err := os.WriteFile(filepath.Join(s.state, name), data, 0400); err != nil {
				return s, err
			}
		}
		env := []string{}
		for _, v := range s.env {
			if !strings.HasPrefix(v, key+"=") {
				env = append(env, v)
			}
		}
		s.env = append(env, key+"="+s.state)
		if s.backend == BackendClaude {
			source := filepath.Join(native, ".claude.json")
			if os.Getenv(key) == "" {
				h, err := os.UserHomeDir()
				if err != nil {
					return s, err
				}
				source = filepath.Join(h, ".claude.json")
			}
			if data, err := os.ReadFile(source); err == nil {
				if err = os.WriteFile(filepath.Join(s.state, ".claude.json"), data, 0600); err != nil {
					return s, err
				}
			} else if !os.IsNotExist(err) {
				return s, err
			}
			env := []string{}
			for _, v := range s.env {
				if !strings.HasPrefix(v, "HOME=") {
					env = append(env, v)
				}
			}
			s.env = append(env, "HOME="+s.state)
			id := opaque()
			s.id = id[:8] + "-" + id[8:12] + "-4" + id[13:16] + "-a" + id[17:20] + "-" + id[20:]
			s.args = append(removeArgs(s.args, "--no-session-persistence"), "--session-id", s.id)
		} else {
			s.args = removeArgs(s.args, "--ephemeral")
		}
	}
	return s, nil
}

func (s reviewSession) finalArgs(stdout []byte) ([]string, bool) {
	switch s.backend {
	case BackendPi:
		args := []string{}
		for i := 0; i < len(s.args); i++ {
			if s.args[i] == "--tools" {
				i++
				continue
			}
			args = append(args, s.args[i])
		}
		return append(args, "--no-tools", "--thinking", "low"), true
	case BackendClaude:
		args := []string{}
		for i := 0; i < len(s.args); i++ {
			if s.args[i] == "--session-id" {
				i++
				continue
			}
			if s.args[i] == "--tools" {
				args = append(args, "--tools", "")
				i++
				continue
			}
			args = append(args, s.args[i])
		}
		return append(args, "--resume", s.id, "--effort", "low"), true
	case BackendCodex:
		id := ""
		jsonLines(stdout, func(event map[string]json.RawMessage) {
			if field(event, "type") == "thread.started" {
				id = field(event, "thread_id")
			}
		})
		if id == "" {
			return nil, false
		}
		args := []string{"exec", "resume"}
		for i := 1; i < len(s.args)-1; i++ {
			if s.args[i] == "--sandbox" {
				i++
				continue
			}
			args = append(args, s.args[i])
		}
		return append(args, "-c", `sandbox_mode="read-only"`, "-c", `model_reasoning_effort="low"`, id, "-"), true
	}
	return nil, false
}

func addUsage(a, b TokenUsage) TokenUsage {
	return TokenUsage{InputTokens: a.InputTokens + b.InputTokens, OutputTokens: a.OutputTokens + b.OutputTokens, CachedInputTokens: a.CachedInputTokens + b.CachedInputTokens, Tokens: a.Tokens + b.Tokens, CostUSD: a.CostUSD + b.CostUSD}
}

func finalReviewAnswer(first, final agentAnswer) agentAnswer {
	if final.Model == "" {
		final.Model = first.Model
	}
	if final.Provider == "" {
		final.Provider = first.Provider
	}
	final.Usage = addUsage(first.Usage, final.Usage)
	final.Tokens, final.CostUSD = final.Usage.Tokens, final.Usage.CostUSD
	return final
}

func runBoundedReview(parent context.Context, x *Execution, dir, tree string) (reviewOutcome, runner.Confinement) {
	s, err := newReviewSession(x, dir)
	if err != nil {
		return reviewOutcome{Status: "inconclusive", Log: "private review session unavailable: " + err.Error()}, runner.Confinement{}
	}
	if x.Config.Backend == BackendCodex {
		if err = os.WriteFile(filepath.Join(dir, "schema.json"), []byte(verdictSchema(x.Full, len(x.Responses) > 0)), 0600); err != nil {
			return reviewOutcome{Status: "inconclusive", Log: "review schema unavailable"}, runner.Confinement{}
		}
	}
	policy, confinement := confinementFor("reviewer", x.Config, []string{s.state, s.temp})
	budget := defaultReviewBudget
	if d, err := time.ParseDuration(x.ReviewBudget); err == nil && d > 0 {
		budget = d
	}
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()
	ctx = runner.WithExecutor(ctx, x.ID, Home)
	ctx, stop := runner.ProcessContext(ctx)
	defer stop()
	deadline, _ := ctx.Deadline()
	exploration, stopExploring := context.WithDeadline(ctx, time.Now().Add(time.Until(deadline)*4/5))
	defer stopExploring()
	run := func(turn context.Context, args []string, input []byte) (reviewOutcome, []byte) {
		exe, argv, err := runner.ConfinedCommand(policy, x.Config.Executable, args)
		if err != nil {
			return reviewOutcome{Status: "inconclusive", Log: "confinement launcher unavailable"}, nil
		}
		out, stderr := runner.NewBuffer(runner.StdoutLimit), runner.NewBuffer(runner.StderrLimit)
		turn, stopTurn := context.WithCancel(turn)
		defer stopTurn()
		code, err := runner.RunProcess(turn, tree, exe, argv, input, s.env, stallWatch(out, x.Config.Backend, stopTurn), stderr)
		_ = appendTranscript(x.Run, fmt.Sprintf("reviewer %s | %s %s | exit=%d", x.ID, s.backend, x.Config.Model, code), out.Bytes(), stderr.Bytes()) // Transcript capture supplements the authoritative bounded execution record.
		result := reviewFrom(x, code, out, stderr, err, executorStop(turn))
		if errors.Is(err, runner.ErrWriterTreeUnknown) {
			result.Answer.Reason = "writer_tree_unknown"
		}
		if reason, failed := runner.ConfinementFailed(code, stderr.Bytes()); failed {
			result.Status = "inconclusive"
			result.Log += "; " + reason
		}
		return result, out.Bytes()
	}
	first, stdout := run(exploration, s.args, x.Input)
	nearLimit := errors.Is(exploration.Err(), context.DeadlineExceeded) && ctx.Err() == nil
	if ctx.Err() == nil && first.Answer.Reason != "writer_tree_unknown" && (nearLimit || first.Answer.Reason == "invalid_output") {
		args, ok := s.finalArgs(stdout)
		if ok {
			data, inputErr := reviewFinalizationInput(x, first, nearLimit)
			if inputErr != nil {
				first.Status = "inconclusive"
				first.Log += "; finalization input invalid: " + inputErr.Error()
				return first, confinement
			}
			final, _ := run(ctx, args, data)
			final.Answer = finalReviewAnswer(first.Answer, final.Answer)
			final.Log = first.Log + "; one finalization turn: " + final.Log
			final.Finalization = true
			if ctx.Err() != nil && parent.Err() == nil && budget < runner.ProcessTimeout(parent) {
				final.Status = "inconclusive"
				final.Answer.Reason = "review_budget"
				final.Answer.Diagnostic = "review wall-time budget exhausted"
			}
			return final, confinement
		}
		first.Log += "; finalization unavailable: backend did not report a resumable session"
	}
	if (ctx.Err() != nil || nearLimit) && parent.Err() == nil && budget < runner.ProcessTimeout(parent) {
		first.Status = "inconclusive"
		first.Answer.Reason = "review_budget"
		first.Answer.Diagnostic = "review wall-time budget exhausted"
	}
	return first, confinement
}

func reviewFinalizationInput(x *Execution, first reviewOutcome, nearLimit bool) ([]byte, error) {
	var original map[string]any
	if err := json.Unmarshal(x.Input, &original); err != nil {
		return nil, fmt.Errorf("decode original review input: %w", err)
	}
	input := map[string]any{"Head": original["Head"], "Paths": original["Paths"], "Contract": original["Contract"]}
	input["Controller"] = "Return your verdict now. Stop exploring. No further tools or investigation. Use the evidence already gathered in this session and the supplied changed hunks. Return only the required JSON object."
	if !nearLimit {
		input["PreviousAnswerError"] = safeMessage(first.Answer.Diagnostic)
	}
	data, err := json.Marshal(input)
	return data, err
}

func hunkContext(files []reviewFile, ranges map[string][][2]int) []reviewFile {
	const contextLines = 20
	for i := range files {
		spans := ranges[files[i].Path]
		if len(spans) == 0 {
			continue
		}
		lines := strings.SplitAfter(files[i].Lines, "\n")
		var excerpt strings.Builder
		omitted := false
		for n, line := range lines {
			if line == "" {
				continue
			}
			number := n + 1
			if prefix, _, ok := strings.Cut(line, ": "); ok {
				if parsed, err := strconv.Atoi(prefix); err == nil {
					number = parsed
				}
			}
			keep := false
			for _, span := range spans {
				if number >= span[0]-contextLines && number <= span[1]+contextLines {
					keep = true
					break
				}
			}
			if keep {
				if omitted {
					excerpt.WriteString("[omitted]\n")
					omitted = false
				}
				excerpt.WriteString(line)
			} else {
				omitted = true
			}
		}
		if omitted {
			excerpt.WriteString("[omitted]\n")
		}
		files[i].Lines = excerpt.String()
	}
	return files
}

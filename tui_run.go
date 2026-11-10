package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const screenIntent = "intent"

type runPlan struct {
	agent  string
	intent string
	target string
}

// runSeq is one explicit Run: collect intent, select the agent, prepare,
// load rules, then start the review or pipeline. Each step sends at most one
// request with the token from the response before it.
type runSeq struct {
	step    string
	cancel  context.CancelFunc
	stopped bool
	intent  string
	source  string
	sent    map[string]bool
}

type intentMsg struct {
	match intentMatch
	err   error
}

type collector func(ctx context.Context, worktree, target, head string) (intentMatch, error)

func localCollector(ctx context.Context, worktree, target, head string) (intentMatch, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return intentMatch{}, err
	}
	resolved, err := committedTarget(worktree, target)
	if err != nil {
		return intentMatch{}, errors.New("the target " + target + " is not a commit here; choose another target")
	}
	return collectIntent(ctx, home, worktree, resolved, head)
}

var runStepLabels = map[string]string{
	"collect":       "Collecting intent from local agent sessions",
	"apply":         "Selecting the agent",
	"prepare":       "Saving intent and target",
	"resolve-rules": "Loading rules",
	"review":        "Starting the review",
	"pipeline":      "Starting the pipeline",
	"question":      "Waiting for the intent",
}

func (m *tuiModel) planAgent() string {
	r := m.v.Run
	switch {
	case m.plan.agent != "":
		return m.plan.agent
	case r.Reviewer != nil:
		return backendOf(*r.Reviewer)
	}
	if backend := m.current().ReviewerBackend; has(m.agents, backend) {
		return backend
	}
	if len(m.agents) > 0 {
		return m.agents[0]
	}
	return ""
}

func (m *tuiModel) planTarget() string {
	switch {
	case m.plan.target != "":
		return m.plan.target
	case m.v.Run.Target != "":
		return m.v.Run.Target
	}
	return m.suggestion
}

func (m *tuiModel) planIntent() string {
	if m.plan.intent != "" {
		return m.plan.intent
	}
	return m.v.Run.Intent
}

func (m *tuiModel) runReview() tea.Cmd {
	switch {
	case m.seq != nil:
		m.say("Run is already in progress; nothing else was sent.")
		return nil
	case m.pending:
		m.say("Still waiting for the previous action; nothing else was sent.")
		return nil
	case !m.fresh:
		m.say("Reconnecting: actions are disabled until fresh state arrives. Nothing was sent.")
		return nil
	case m.planAgent() == "":
		m.say("No supported agent is installed.")
		return nil
	case m.planTarget() == "":
		m.openSetup(2)
		m.say("Choose the Git target to review against, then Run again.")
		return nil
	}
	m.seq = &runSeq{intent: m.planIntent(), sent: map[string]bool{}}
	m.message, m.failed = "", false
	return tea.Batch(m.advance(), m.tick())
}

func (m *tuiModel) advance() tea.Cmd {
	seq := m.seq
	if seq == nil {
		return nil
	}
	if seq.stopped {
		m.seq = nil
		m.say("Run stopped after " + strings.ToLower(runStepLabels[seq.step]) + ". Nothing else was sent.")
		return nil
	}
	if !m.fresh {
		m.seq = nil
		m.say("Run stopped: the connection to the daemon was lost. Nothing else was sent; Run again once the screen is live.")
		return nil
	}
	r, settings := m.v.Run, m.current()
	agent, target := m.planAgent(), m.planTarget()
	full := settings.Preset == "full"
	allowed := func(op string) bool { return has(m.v.Actions, op) }
	switch {
	case seq.intent == "":
		return m.collect(target)
	case agent != settings.ReviewerBackend && (r.Reviewer == nil || backendOf(*r.Reviewer) != agent):
		draft := settings
		draft.ReviewerBackend, draft.ReviewerModel = agent, ""
		q := mutation(m.v, "apply")
		q.Settings, q.Reason = &draft, "select installed agent"
		return m.step("apply", q)
	case r.Reviewer == nil || r.Intent != seq.intent || r.Target == "" || m.plan.target != "" && !sameTarget(r, m.plan.target):
		q := mutation(m.v, "prepare")
		q.Intent, q.Target = seq.intent, target
		return m.step("prepare", q)
	case !m.v.RulesReady:
		return m.step("resolve-rules", mutation(m.v, "resolve-rules"))
	case !full:
		return m.step("review", mutation(m.v, "review"))
	case allowed("pipeline"):
		return m.step("pipeline", mutation(m.v, "pipeline"))
	}
	m.seq = nil
	m.say("Run cannot start the pipeline in the current state. Nothing else was sent.")
	return nil
}

func sameTarget(r *Run, target string) bool {
	resolved, err := committedTarget(r.Submitted.Worktree, target)
	return err == nil && resolved == r.Target
}

func (m *tuiModel) step(op string, q Request) tea.Cmd {
	m.seq.step = op
	if m.seq.sent[op] {
		m.seq = nil
		m.say("Run stopped: " + strings.ToLower(runStepLabels[op]) + " did not change the run state. Nothing else was sent.")
		return nil
	}
	m.seq.sent[op] = true
	if !has(m.v.Actions, op) {
		label := runStepLabels[op]
		m.seq = nil
		m.say("Run stopped: " + strings.ToLower(label) + " is not available in the current state. Nothing was sent.")
		return nil
	}
	m.pending = true
	client := m.client
	return func() tea.Msg {
		v, err := client.send(q, false)
		return responseMsg{op: op, label: runStepLabels[op], v: v, err: err, run: true}
	}
}

func (m *tuiModel) collect(target string) tea.Cmd {
	r := m.v.Run
	ctx, cancel := context.WithCancel(context.Background())
	m.seq.step, m.seq.cancel = "collect", cancel
	collect, worktree, head := m.collector, r.Submitted.Worktree, r.Candidate
	return func() tea.Msg {
		match, err := collect(ctx, worktree, target, head)
		return intentMsg{match, err}
	}
}

func (m *tuiModel) collected(msg intentMsg) tea.Cmd {
	seq := m.seq
	if seq == nil || seq.step != "collect" {
		return nil
	}
	seq.cancel()
	switch {
	case seq.stopped || errors.Is(msg.err, context.Canceled):
		m.seq = nil
		m.say("Intent collection cancelled. Nothing was sent.")
		return nil
	case msg.err == nil && msg.match.Session != nil:
		return m.useSession(msg.match.Session)
	case msg.err == nil && len(msg.match.Candidates) > 0:
		m.candidates = msg.match.Candidates
		m.cursor = 0
		m.question = "Several agent sessions in this worktree mention the changed files. Which one asked for this change?"
	default:
		m.candidates = nil
		reason := "No local agent session started in this worktree mentions the changed files"
		if msg.err != nil && !errors.Is(msg.err, errNoIntent) {
			reason = "Intent collection failed: " + msg.err.Error()
		}
		m.question = reason + ". What should this change do?"
		m.edit("", "the change in your own words")
	}
	seq.step = "question"
	m.screen = screenIntent
	return nil
}

// useSession asks for a shorter intent instead of cutting requests that do
// not fit the controller's limit.
func (m *tuiModel) useSession(s *intentSession) tea.Cmd {
	intent, ok := s.Intent()
	if !ok {
		m.candidates = nil
		m.question = fmt.Sprintf("The requests in %s are too long for a review intent: %d bytes, at most %d. Nothing was cut or sent. Write the intent in your own words, keeping every requirement.", s.Label(), len(intent), intentLimit)
		m.seq.step, m.screen = "question", screenIntent
		m.edit("", "the change in your own words")
		return nil
	}
	m.seq.intent, m.seq.source, m.plan.intent = intent, s.Label(), intent
	m.home("")
	return m.advance()
}

func (m *tuiModel) stopRun() {
	if m.seq == nil {
		return
	}
	if m.seq.cancel != nil {
		m.seq.cancel()
	}
	if m.pending || m.seq.step == "collect" {
		m.seq.stopped = true
		m.message, m.failed = "Stopping Run after the current step…", false
		return
	}
	m.seq = nil
	m.home("")
	m.say("Run stopped. Nothing else was sent.")
}

func (m *tuiModel) intentKey(key string) tea.Cmd {
	rows := len(m.candidates) + 1
	switch key {
	case "esc":
		m.stopRun()
	case "down", "j", "tab":
		m.cursor = (m.cursor + 1) % rows
	case "up", "k", "shift+tab":
		m.cursor = (m.cursor + rows - 1) % rows
	case "enter":
		if m.cursor == len(m.candidates) {
			m.candidates = nil
			m.edit("", "the change in your own words")
			return nil
		}
		return m.useSession(m.candidates[m.cursor])
	}
	return nil
}

func (m *tuiModel) intentTyped(accept bool, value string) tea.Cmd {
	if !accept {
		m.editing = false
		m.input.Blur()
		m.stopRun()
		return nil
	}
	if !validText(value) {
		m.say(fmt.Sprintf("Describe the change in at most %d bytes, or press Esc to stop Run.", intentLimit))
		return nil
	}
	m.editing = false
	m.input.Blur()
	m.seq.intent, m.plan.intent = value, value
	m.home("")
	return m.advance()
}

func (m *tuiModel) intentLabel() (string, string) {
	intent := m.planIntent()
	switch {
	case intent == "":
		return stepTodo, "collected from your agent session on Run"
	case strings.HasPrefix(intent, collectedPrefix):
		state := stepTodo
		if m.v.Run.Intent == intent {
			state = stepDone
		}
		return state, intentSummary(intent)
	case m.v.Run.Intent == intent:
		return stepDone, intent
	}
	return stepTodo, intent + " · typed"
}

const collectedPrefix = "User request collected from "

func intentSummary(intent string) string {
	if !strings.HasPrefix(intent, collectedPrefix) {
		return intent
	}
	source, body, _ := strings.Cut(strings.TrimPrefix(intent, collectedPrefix), ". These are the user's own words from that session, not a summary:\n\n")
	line, _, _ := strings.Cut(body, "\n")
	return "“" + line + "” · collected from " + source
}

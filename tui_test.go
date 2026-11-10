package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type recordingClient struct {
	requests []Request
	answer   func(q Request) (Response, error)
}

func (c *recordingClient) send(q Request, spawn bool) (Response, error) {
	c.requests = append(c.requests, q)
	if c.answer == nil {
		return Response{Outcome: "ok"}, nil
	}
	return c.answer(q)
}

func (c *recordingClient) mutations() []Request {
	var out []Request
	for _, q := range c.requests {
		if q.Op != "models" && q.Op != "inspect" && q.Op != "transcript" {
			out = append(out, q)
		}
	}
	return out
}

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// drive feeds messages and runs the commands they return, as the Bubble Tea
// runtime would, and reports whether the program asked to quit.
func drive(m *tuiModel, msgs ...tea.Msg) (quit bool) {
	queue := append([]tea.Msg{}, msgs...)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		_, cmd := m.Update(msg)
		next := run(cmd)
		for _, n := range next {
			if _, ok := n.(tea.QuitMsg); ok {
				quit = true
				continue
			}
			queue = append(queue, n)
		}
	}
	return quit
}

func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, run(c)...)
		}
		return out
	case spinner.TickMsg, nil:
		return nil
	}
	return []tea.Msg{msg}
}

func press(m *tuiModel, keys ...string) (quit bool) {
	for _, key := range keys {
		if drive(m, keyMsg(key)) {
			quit = true
		}
	}
	return quit
}

func typed(text string) []string {
	var keys []string
	for _, r := range text {
		if r == ' ' {
			keys = append(keys, "space")
		} else {
			keys = append(keys, string(r))
		}
	}
	return keys
}

func tuiState(status string, actions ...string) Response {
	r := tuiRun(status)
	r.Submitted = Identity{Branch: "refs/heads/feat", Head: strings.Repeat("a", 40)}
	return Response{Outcome: "ok", RunID: r.ID, Input: r.Input, Token: r.Token, Gate: "none", Run: r, Actions: append([]string{"attach", "inspect"}, actions...)}
}

func ready(v Response) Response {
	v.Run.Intent, v.Run.Target, v.Run.Reviewer = "fix it", strings.Repeat("b", 40), &ReviewerConfig{Backend: "claude"}
	v.RulesReady = true
	return v
}

func newTestTUI(v Response, client *recordingClient, agents ...string) *tuiModel {
	m := newTUI(client, v, agents, "main", nil)
	drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

// fakeController answers like the daemon for the Run steps: each accepted
// mutation changes the run and issues a new token.
func fakeController(v Response) *recordingClient {
	state := v
	n := 0
	client := &recordingClient{}
	client.answer = func(q Request) (Response, error) {
		if q.Expected != state.Token || q.Action == "" {
			refused := state
			refused.Outcome, refused.Error = "refused", "stale run/input/gate/token"
			return refused, nil
		}
		next := state
		run := *state.Run
		next.Run = &run
		n++
		next.Token = fmt.Sprint("token-", n)
		run.Token = next.Token
		switch q.Op {
		case "apply":
			run.Configs = append(run.Configs, Snapshot{Settings: *q.Settings})
		case "prepare":
			run.Intent, run.Target, run.Reviewer = q.Intent, strings.Repeat("b", 40), &ReviewerConfig{Backend: run.Configs[len(run.Configs)-1].Settings.ReviewerBackend}
			next.Actions = append(next.Actions, "review")
		case "resolve-rules":
			next.RulesReady = true
		case "review":
			run.Status, run.ReviewUsed = "running", 1
			next.Actions = []string{"attach", "inspect", "cancel"}
		}
		state = next
		return next, nil
	}
	return client
}

func ops(requests []Request) []string {
	var out []string
	for _, q := range requests {
		out = append(out, q.Op)
	}
	return out
}

func collectedFor(t *testing.T, session *intentSession) collector {
	return func(ctx context.Context, worktree, target, head string) (intentMatch, error) {
		if session == nil {
			t.Fatal("Run collected intent although one was typed")
		}
		return intentMatch{Session: session}, nil
	}
}

func firstRun() Response {
	v := tuiState("unconfigured", "apply", "save", "resolve-rules", "prepare", "new")
	v.Run.Submitted.Worktree = "."
	v.Run.Configs[0].Settings.ReviewerBackend = "pi"
	return v
}

func TestOneRunCollectsIntentPreparesLoadsRulesAndStartsReview(t *testing.T) {
	client := fakeController(firstRun())
	m := newTestTUI(firstRun(), client, "pi", "claude")
	session := &intentSession{Agent: "claude", ID: "abcdef123456", Requests: []string{"Add retry with jitter"}}
	m.collector = collectedFor(t, session)
	if primaryAction(m.v, m.agents).Label != "Run review" {
		t.Fatalf("first screen primary: %+v", primaryAction(m.v, m.agents))
	}
	press(m, "enter")
	got := client.mutations()
	if strings.Join(ops(got), ",") != "prepare,resolve-rules,review" {
		t.Fatalf("Run sent %v", ops(got))
	}
	if got[0].Intent != session.Intent() || got[0].Target != "main" || got[0].Expected != "token" {
		t.Fatalf("prepare: %+v", got[0])
	}
	if got[1].Expected != "token-1" || got[2].Expected != "token-2" || got[0].Action == got[1].Action || got[1].Action == got[2].Action {
		t.Fatalf("Run steps must use the previous response token and distinct action IDs: %+v", got)
	}
	if m.seq != nil || m.v.Run.Status != "running" || !strings.Contains(m.message, "Review started") {
		t.Fatalf("Run did not finish at the started review: %q %s", m.message, m.v.Run.Status)
	}
	press(m, "enter")
	if len(client.mutations()) != 3 {
		t.Fatal("a second Enter after start sent another request")
	}
}

func TestEditedAgentAndTypedIntentAreUsedByRun(t *testing.T) {
	client := fakeController(firstRun())
	m := newTestTUI(firstRun(), client, "pi", "claude")
	m.collector = collectedFor(t, nil)
	press(m, "tab", "enter", "down", "enter")
	if quit := press(m, typed("fix q x retry")...); quit {
		t.Fatal("typing q inside the intent detached")
	}
	press(m, "enter")
	if m.input.Value() != "main" {
		t.Fatalf("target not prefilled with the suggestion: %q", m.input.Value())
	}
	press(m, "enter", "enter")
	if len(client.mutations()) != 0 || !strings.Contains(m.message, "Nothing has run yet") {
		t.Fatalf("editing sent requests: %v %q", ops(client.mutations()), m.message)
	}
	press(m, "enter")
	got := client.mutations()
	if strings.Join(ops(got), ",") != "apply,prepare,resolve-rules,review" {
		t.Fatalf("Run sent %v", ops(got))
	}
	if got[0].Settings.ReviewerBackend != "claude" || got[1].Intent != "fix q x retry" {
		t.Fatalf("Run ignored the edits: %+v %+v", got[0].Settings, got[1])
	}
}

func TestRunAsksOnceWhenNoSessionMatchesOrSeveralDo(t *testing.T) {
	client := fakeController(firstRun())
	m := newTestTUI(firstRun(), client, "pi")
	m.collector = func(ctx context.Context, worktree, target, head string) (intentMatch, error) {
		return intentMatch{}, errNoIntent
	}
	press(m, "enter")
	if m.screen != screenIntent || len(client.mutations()) != 0 || !strings.Contains(m.View(), "What should this change do?") {
		t.Fatalf("no-match did not ask: %s\n%s", m.screen, m.View())
	}
	press(m, "esc")
	if m.seq != nil || len(client.mutations()) != 0 {
		t.Fatal("Esc at the question did not stop Run")
	}
	press(m, "enter")
	press(m, typed("typed answer")...)
	press(m, "enter")
	if got := client.mutations(); len(got) != 3 || got[0].Intent != "typed answer" {
		t.Fatalf("typed answer not used: %+v", got)
	}

	client = fakeController(firstRun())
	m = newTestTUI(firstRun(), client, "pi")
	a := &intentSession{Agent: "pi", ID: "aaaaaaaa1", Requests: []string{"first"}}
	b := &intentSession{Agent: "codex", ID: "bbbbbbbb2", Requests: []string{"second"}}
	m.collector = func(ctx context.Context, worktree, target, head string) (intentMatch, error) {
		return intentMatch{Candidates: []*intentSession{a, b}}, nil
	}
	press(m, "enter")
	if m.screen != screenIntent || !strings.Contains(m.View(), "second") {
		t.Fatalf("ambiguity not shown:\n%s", m.View())
	}
	press(m, "down", "enter")
	if got := client.mutations(); len(got) != 3 || got[0].Intent != b.Intent() {
		t.Fatalf("chosen session not used: %+v", got)
	}
}

func TestRunCancelAndDuplicateEnterSendNothingExtra(t *testing.T) {
	client := fakeController(firstRun())
	m := newTestTUI(firstRun(), client, "pi")
	m.collector = func(ctx context.Context, worktree, target, head string) (intentMatch, error) {
		<-ctx.Done()
		return intentMatch{}, ctx.Err()
	}
	_, collect := m.Update(keyMsg("enter"))
	if _, again := m.Update(keyMsg("enter")); again != nil {
		t.Fatal("a second Enter during Run started more work")
	}
	press(m, "x")
	for _, msg := range run(collect) {
		drive(m, msg)
	}
	if m.seq != nil || len(client.mutations()) != 0 || !strings.Contains(m.message, "cancelled") {
		t.Fatalf("cancel during collection: %q %v", m.message, ops(client.mutations()))
	}
}

func TestRunStopsOnRefusalWithoutRetry(t *testing.T) {
	client := &recordingClient{answer: func(q Request) (Response, error) {
		refused := firstRun()
		refused.Outcome, refused.Error, refused.Token = "refused", "source head or branch moved", "fresh"
		refused.Actions = []string{"attach", "inspect", "new"}
		return refused, nil
	}}
	m := newTestTUI(firstRun(), client, "pi")
	m.collector = collectedFor(t, &intentSession{Agent: "pi", ID: "x", Requests: []string{"r"}})
	press(m, "enter")
	if got := client.mutations(); len(got) != 1 || got[0].Op != "prepare" {
		t.Fatalf("refused Run step retried or continued: %v", ops(got))
	}
	if m.seq != nil || !strings.Contains(m.message, "source head or branch moved") || !strings.Contains(m.message, "nothing was retried") {
		t.Fatalf("refusal not explained: %q", m.message)
	}
	if primaryAction(m.v, m.agents).ID != "new" {
		t.Fatalf("primary after a moved source: %+v", primaryAction(m.v, m.agents))
	}
}

func TestNavigationSendsNoMutation(t *testing.T) {
	v := ready(tuiState("outstanding-findings", "apply", "save", "resolve-rules", "repair", "new", "rerun-from-start"))
	v.Run.Findings = []Finding{{ID: "f1", Priority: "P1", File: "a.go", Start: 3, Explanation: "broken", Action: "fix", Head: v.Run.Candidate}}
	v.Run.Review = &Execution{Result: &Verdict{Findings: v.Run.Findings}}
	client := &recordingClient{}
	m := newTestTUI(v, client, "claude")
	before := digest(v)
	press(m, "tab", "down", "up", "esc", "?", "down", "esc", "i", "c", "esc", "a", "down", "down", "esc", "f", "enter", "down", "esc", "esc", "s", "right", "down", "esc")
	drive(m, tea.WindowSizeMsg{Width: 56, Height: 24}, tea.WindowSizeMsg{Width: 100, Height: 24})
	if got := client.mutations(); len(got) != 0 {
		t.Fatalf("navigation sent mutations: %+v", got)
	}
	if digest(m.v) != before {
		t.Fatal("navigation changed the shown controller state")
	}
	if !m.draftChanged() {
		t.Fatal("settings draft was lost when leaving Settings")
	}
}

func TestReconnectDisablesMutationsUntilFreshState(t *testing.T) {
	v := ready(tuiState("unconfigured", "apply", "save", "resolve-rules", "prepare", "review", "new"))
	client := &recordingClient{}
	m := newTestTUI(v, client, "claude")
	drive(m, liveUpdate{state: "reconnecting (daemon unavailable)"})
	press(m, "enter")
	if got := client.mutations(); len(got) != 0 {
		t.Fatalf("mutation sent while reconnecting: %+v", got)
	}
	if !strings.Contains(m.View(), "RECONNECTING") || !strings.Contains(m.message, "Reconnecting") {
		t.Fatalf("reconnecting state not shown:\n%s\n%s", m.View(), m.message)
	}
	fresh := v
	fresh.Stream, fresh.Sequence = "s2", 1
	drive(m, liveUpdate{state: "live", view: &fresh})
	press(m, "enter")
	got := client.mutations()
	if len(got) != 1 || got[0].Op != "review" {
		t.Fatalf("explicit start review after resync: %+v", got)
	}
}

func TestOlderWatchAnswerDoesNotReplaceNewerState(t *testing.T) {
	v := ready(tuiState("running", "cancel"))
	v.Stream, v.Sequence = "s", 5
	m := newTestTUI(v, &recordingClient{}, "claude")
	old := ready(tuiState("unconfigured", "review"))
	old.Stream, old.Sequence = "s", 4
	drive(m, liveUpdate{state: "live", view: &old})
	if m.v.Run.Status != "running" {
		t.Fatal("an older watch answer replaced newer state")
	}
}

func TestStaleRefusalRefreshesWithoutRetry(t *testing.T) {
	v := ready(tuiState("unconfigured", "apply", "save", "resolve-rules", "prepare", "review", "new"))
	client := &recordingClient{}
	client.answer = func(q Request) (Response, error) {
		refused := ready(tuiState("unconfigured", "apply", "save", "resolve-rules", "prepare", "new"))
		refused.Outcome, refused.Error, refused.Token = "refused", "stale run/input/gate/token", "fresh"
		refused.RulesReady = false
		return refused, nil
	}
	m := newTestTUI(v, client, "claude")
	press(m, "enter")
	if got := client.mutations(); len(got) != 1 {
		t.Fatalf("refusal retried: %+v", got)
	}
	if m.v.Token != "fresh" || !strings.Contains(m.message, "stale run/input/gate/token") || !strings.Contains(m.message, "nothing was retried") {
		t.Fatalf("refusal not explained with refreshed state: %q token %q", m.message, m.v.Token)
	}
	if primaryAction(m.v, m.agents).ID != "run" || !strings.Contains(primaryAction(m.v, m.agents).Hint, "saved intent") {
		t.Fatalf("primary action not rederived from refreshed state: %+v", primaryAction(m.v, m.agents))
	}
}

func TestDetachIsNotCancel(t *testing.T) {
	v := ready(tuiState("running", "cancel"))
	v.Run.Review = &Execution{ID: "x", Backend: "claude"}
	for _, key := range []string{"q", "ctrl+c"} {
		client := &recordingClient{}
		m := newTestTUI(v, client, "claude")
		if !press(m, key) || len(client.requests) != 0 {
			t.Fatalf("%s did not detach cleanly: %+v", key, client.requests)
		}
		if !strings.Contains(m.farewell, "keeps running") {
			t.Fatalf("detach notice does not say work continues: %q", m.farewell)
		}
	}
	client := &recordingClient{}
	m := newTestTUI(v, client, "claude")
	press(m, "x")
	if len(client.requests) != 0 || m.screen != screenConfirm {
		t.Fatal("one x cancelled without confirmation")
	}
	press(m, "esc", "x", "q")
	if len(client.requests) != 0 {
		t.Fatalf("q at the cancel confirmation cancelled: %+v", client.requests)
	}
	m = newTestTUI(v, client, "claude")
	press(m, "x", "x")
	if got := client.mutations(); len(got) != 1 || got[0].Op != "cancel" {
		t.Fatalf("x x did not send one cancel: %+v", got)
	}
}

func TestTypedConfirmationForNewRun(t *testing.T) {
	v := tuiState("clean-review", "apply", "save", "resolve-rules", "new", "rerun-from-start")
	client := &recordingClient{}
	m := newTestTUI(v, client, "claude")
	press(m, "a")
	items := menuItems(m.v, true)
	at := slices.IndexFunc(items, func(a tuiAction) bool { return a.ID == "new" })
	for i := 0; i < at; i++ {
		press(m, "down")
	}
	press(m, "enter")
	press(m, typed("nwe")...)
	press(m, "enter")
	if len(client.mutations()) != 0 {
		t.Fatal("mistyped confirmation sent New")
	}
	press(m, "backspace", "backspace", "backspace")
	press(m, typed("new")...)
	press(m, "enter")
	if got := client.mutations(); len(got) != 1 || got[0].Op != "new" {
		t.Fatalf("typed new: %+v", got)
	}
}

func TestDecisionSendsChosenSubsetAndReason(t *testing.T) {
	v := ready(tuiState("needs-decision", "apply", "decide", "new"))
	v.Gate = "gate-1"
	v.Decision = &Decision{ID: "gate-1", Kind: "findings", Stage: "review", Options: []string{"repair", "accept-head"}, Findings: []string{"f1", "f2"}, Reason: "two findings"}
	client := &recordingClient{}
	m := newTestTUI(v, client, "claude")
	press(m, "enter", "enter", "space", "enter")
	press(m, typed("only the first")...)
	press(m, "enter")
	got := client.mutations()
	if len(got) != 1 || got[0].Op != "decide" || got[0].Choice != "repair" || got[0].Gate != "gate-1" || got[0].Reason != "only the first" || !slices.Equal(got[0].Selected, []string{"f2"}) {
		t.Fatalf("decision request: %+v", got)
	}
	client = &recordingClient{}
	m = newTestTUI(v, client, "claude")
	press(m, "enter", "enter", "enter")
	press(m, typed("whole group")...)
	press(m, "enter")
	if got := client.mutations(); len(got) != 1 || got[0].Selected != nil {
		t.Fatalf("whole group must send no selection: %+v", got)
	}
}

func TestSettingsDraftStaysLocalUntilApplyOrSave(t *testing.T) {
	v := tuiState("unconfigured", "apply", "save", "resolve-rules", "prepare", "new")
	client := &recordingClient{}
	m := newTestTUI(v, client, "claude")
	press(m, "s", "right", "down", "down", "enter", "backspace")
	press(m, typed("7")...)
	press(m, "enter", "esc")
	if len(client.mutations()) != 0 {
		t.Fatalf("draft edits sent requests: %+v", client.mutations())
	}
	press(m, "s")
	for range settingNames {
		press(m, "down")
	}
	press(m, "enter")
	got := client.mutations()
	if len(got) != 1 || got[0].Op != "apply" || got[0].Settings.Mode != "ask" || got[0].Settings.Limit != 7 || got[0].Target != "" {
		t.Fatalf("apply request: %+v", got)
	}
	client.requests = nil
	for range settingNames {
		press(m, "up")
	}
	press(m, "left")
	for i := 0; i <= len(settingNames); i++ {
		press(m, "down")
	}
	press(m, "enter")
	got = client.mutations()
	if len(got) != 1 || got[0].Op != "save" || got[0].Target != "user" || got[0].Settings.Mode != "yolo" {
		t.Fatalf("save request: %+v %+v", got, got[0].Settings)
	}
}

func TestFindingsRepairSendsSelectedIDs(t *testing.T) {
	v := ready(tuiState("outstanding-findings", "apply", "save", "resolve-rules", "repair", "new"))
	v.Run.Findings = []Finding{{ID: "f1", Priority: "P1", File: "a.go", Start: 3, Explanation: "broken", Action: "fix", Head: v.Run.Candidate}, {ID: "f2", Priority: "P2", File: "b.go", Start: 9, Explanation: "also broken", Action: "fix", Head: v.Run.Candidate}}
	v.Run.Review = &Execution{Result: &Verdict{Findings: v.Run.Findings}}
	client := &recordingClient{}
	m := newTestTUI(v, client, "claude")
	press(m, "enter", "down", "space", "r")
	if len(client.mutations()) != 0 || m.screen != screenConfirm {
		t.Fatal("repair sent without its confirmation")
	}
	press(m, "enter")
	if got := client.mutations(); len(got) != 1 || got[0].Op != "repair" || !slices.Equal(got[0].Selected, []string{"f2"}) {
		t.Fatalf("repair request: %+v", got)
	}
	without := v
	without.Actions = []string{"attach", "inspect", "apply", "save", "resolve-rules", "new"}
	client = &recordingClient{}
	m = newTestTUI(without, client, "claude")
	press(m, "f", "space", "r")
	if len(client.mutations()) != 0 {
		t.Fatal("repair sent while the controller does not allow it")
	}
}

func TestViewFitsSupportedSizes(t *testing.T) {
	long := strings.Repeat("a long explanation ", 12)
	findings := ready(tuiState("outstanding-findings", "apply", "save", "resolve-rules", "repair", "new"))
	for i := 0; i < 12; i++ {
		findings.Run.Findings = append(findings.Run.Findings, Finding{ID: fmt.Sprint("f", i), Priority: "P1", File: "very/long/path/to/a/file.go", Start: i, Explanation: long, Action: "fix", Head: findings.Run.Candidate})
	}
	findings.Run.Review = &Execution{Result: &Verdict{Findings: findings.Run.Findings}}
	states := map[string]Response{
		"first":    tuiState("unconfigured", "apply", "save", "resolve-rules", "prepare", "new"),
		"running":  ready(tuiState("running", "cancel")),
		"findings": findings,
	}
	for name, v := range states {
		for _, size := range [][2]int{{56, 24}, {80, 24}, {100, 24}} {
			m := newTestTUI(v, &recordingClient{}, "claude")
			drive(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for _, keys := range [][]string{nil, {"f"}, {"f", "enter"}, {"s"}, {"i"}, {"?"}, {"a"}} {
				m.home("")
				press(m, keys...)
				lines := strings.Split(m.View(), "\n")
				if len(lines) > size[1] {
					t.Errorf("%s %v at %dx%d: %d lines", name, keys, size[0], size[1], len(lines))
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Errorf("%s %v at %dx%d: line too wide: %q", name, keys, size[0], size[1], line)
					}
				}
			}
		}
	}
}

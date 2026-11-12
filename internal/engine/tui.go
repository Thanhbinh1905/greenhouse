package engine

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

type tuiClient interface {
	send(q Request, spawn bool) (Response, error)
}

type daemonClient struct{ home, path string }

func (c daemonClient) send(q Request, spawn bool) (Response, error) {
	q.Path = c.path
	return request(c.home, q, spawn)
}

func interactive() bool {
	t := os.Getenv("TERM")
	return t != "" && t != "dumb" && term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

func terminal(h, path string) error {
	if interactive() {
		return tuiTerminal(h, path)
	}
	return lineTerminal(h, path)
}

func tuiTerminal(h, path string) error {
	v, err := call(h, Request{Version: protocolMax, Op: "attach", Path: path})
	if err != nil {
		return err
	}
	if err = protocolRefusal(v); err != nil {
		return err
	}
	if v.Run == nil {
		return errors.New(v.Error)
	}
	updates := make(chan liveUpdate, 1)
	stop := make(chan struct{})
	go follow(h, path, v.Stream, v.Sequence, updates, stop)
	m := newTUI(daemonClient{h, path}, v, installedAgents(), repository.SuggestTarget(path), updates)
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	close(stop)
	if err != nil {
		return err
	}
	if done, ok := final.(*tuiModel); ok && done.farewell != "" {
		fmt.Println(done.farewell)
	}
	return nil
}

type responseMsg struct {
	op    string
	label string
	v     Response
	err   error
	run   bool
}
type modelsMsg struct {
	models []string
	err    string
}
type transcriptMsg struct {
	v   Response
	err error
}

const (
	screenHome     = "home"
	screenMenu     = "menu"
	screenSetup    = "setup"
	screenFindings = "findings"
	screenDecision = "decision"
	screenRecovery = "recovery"
	screenSettings = "settings"
	screenPage     = "page"
	screenConfirm  = "confirm"
)

type tuiModel struct {
	client     tuiClient
	updates    chan liveUpdate
	v          Response
	live       string
	fresh      bool
	pending    bool
	agents     []string
	suggestion string
	width      int
	height     int
	screen     string
	focus      int
	cursor     int
	scroll     int
	message    string
	failed     bool
	farewell   string

	plan       runPlan
	seq        *runSeq
	collector  collector
	candidates []*intentSession
	question   string

	setupStep   int
	agentPick   int
	setupValues [3]string
	input       textinput.Model

	selected  map[string]bool
	expanded  bool
	decision  struct{ step, choice int }
	recovery  struct{ step, choice int }
	limitText string

	draft     *Settings
	editing   bool
	models    []string
	modelsErr string

	confirm  tuiAction
	confirmQ Request
	back     string

	pageTitle string
	pageText  string
	pageKind  string
	criteria  bool

	spin       spinner.Model
	ticking    bool
	runKey     string
	runSince   time.Time
	runFromTop bool
	order      snapshotOrder
	now        func() time.Time
}

func newTUI(client tuiClient, v Response, agents []string, suggestion string, updates chan liveUpdate) *tuiModel {
	input := textinput.New()
	input.Prompt = "> "
	input.Cursor.SetMode(cursor.CursorStatic)
	m := &tuiModel{client: client, updates: updates, live: "live", fresh: true, agents: agents, suggestion: suggestion, width: 80, height: 24, screen: screenHome, input: input, selected: map[string]bool{}, order: snapshotOrder{}, spin: spinner.New(spinner.WithSpinner(spinner.MiniDot)), now: time.Now, collector: localCollector}
	m.setView(v, false)
	return m
}

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(m.waitUpdate(), m.tick())
}

func (m *tuiModel) waitUpdate() tea.Cmd {
	if m.updates == nil {
		return nil
	}
	ch := m.updates
	return func() tea.Msg { return <-ch }
}

func (m *tuiModel) tick() tea.Cmd {
	if m.ticking || !busy(m.v.Run) && m.seq == nil {
		return nil
	}
	m.ticking = true
	return m.spin.Tick
}

func activeExecution(r *Run) string {
	if r == nil || !busy(r) {
		return ""
	}
	key := r.ID + "/" + r.Status
	switch {
	case r.Pipeline != nil && r.Pipeline.Active != nil:
		key += "/" + r.Pipeline.Active.ID
	case r.Status == "writing" && r.Repair != nil:
		key += "/" + r.Repair.ID
	case r.Review != nil:
		key += "/" + r.Review.ID
	}
	return key
}

// setView replaces the shown state. seen reports that an earlier state was
// already on screen, so a newly running execution started while watched.
func (m *tuiModel) setView(v Response, seen bool) {
	if m.v.Run != nil && v.Run != nil && m.v.Run.ID != v.Run.ID {
		m.selected = map[string]bool{}
	}
	if seen && !m.failed && m.v.Run != nil && v.Run != nil && m.v.Run.Status != v.Run.Status {
		m.message = ""
	}
	m.order.adopt(v, m.v)
	m.v = v
	if key := activeExecution(v.Run); key != m.runKey {
		m.runKey, m.runSince, m.runFromTop = key, m.now(), seen
	}
	r := v.Run
	switch m.screen {
	case screenDecision:
		if v.Decision == nil || !has(v.Actions, "decide") {
			m.home("The decision is no longer open.")
		}
	case screenRecovery:
		if r == nil || r.Recovery == nil || !has(v.Actions, "recover") {
			m.home("Recovery is no longer required.")
		}
	case screenSetup:
		if !has(v.Actions, "prepare") {
			m.home("Setup is no longer available for this run.")
		}
	case screenConfirm:
		if m.confirm.Op != "" && m.confirm.Op != "trust" && !has(v.Actions, m.confirm.Op) {
			m.home(m.confirm.Label + " is no longer available.")
		}
	}
	if n := len(homeTargets(m)); m.focus >= n {
		m.focus = 0
	}
}

func (m *tuiModel) home(message string) {
	m.screen, m.focus, m.editing = screenHome, 0, false
	m.input.Blur()
	if message != "" {
		m.message, m.failed = message, false
	}
}

func (m *tuiModel) say(message string) {
	m.message, m.failed = message, true
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	if width := m.fieldWidth(); width != m.input.Width {
		m.input.Width = width
		m.input.SetCursor(m.input.Position())
	}
	return model, cmd
}

func (m *tuiModel) ordered(v Response, actionResponse bool) bool {
	return v.Run != nil && m.order.accepts(v, m.v, actionResponse)
}

func (m *tuiModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case liveUpdate:
		if msg.view != nil && m.ordered(*msg.view, false) {
			m.setView(*msg.view, true)
		}
		m.live = msg.state
		m.fresh = msg.state == "live" && msg.view != nil
		return m, tea.Batch(m.waitUpdate(), m.tick())
	case spinner.TickMsg:
		if !busy(m.v.Run) && m.seq == nil {
			m.ticking = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case responseMsg:
		if msg.run {
			return m, m.runAnswered(msg)
		}
		return m, m.answered(msg)
	case intentMsg:
		return m, m.collected(msg)
	case modelsMsg:
		m.models, m.modelsErr = msg.models, msg.err
		return m, nil
	case transcriptMsg:
		switch {
		case msg.err != nil:
			m.say("Transcript unavailable: " + msg.err.Error())
		case msg.v.Transcript == nil:
			m.say("Transcript unavailable: " + msg.v.Error)
		default:
			m.openPage("Transcript log", "logs", transcriptPage(m.v.RunID, msg.v.Transcript))
		}
		return m, nil
	case tea.KeyMsg:
		return m, m.key(msg)
	}
	return m, nil
}

func (m *tuiModel) answered(msg responseMsg) tea.Cmd {
	m.pending = false
	if msg.err != nil {
		m.fresh, m.live = false, "reconnecting (daemon unavailable)"
		m.say(msg.label + " was not confirmed: " + msg.err.Error() + ". Actions stay disabled until fresh state arrives.")
		return nil
	}
	if m.ordered(msg.v, true) {
		m.setView(msg.v, true)
	}
	if msg.v.Outcome != "ok" {
		m.say(msg.label + " refused: " + msg.v.Error + ". The screen shows the controller's current state; nothing was retried.")
		return m.tick()
	}
	m.message, m.failed = doneMessage(msg.op, msg.label, msg.v), false
	switch msg.op {
	case "apply", "save":
		m.draft = nil
		if m.screen == screenSettings {
			m.openSettings(m.cursor)
		}
	case "repair":
		m.selected = map[string]bool{}
	}
	if m.screen != screenSettings {
		m.home(m.message)
	}
	return m.tick()
}

// runAnswered handles one Run step. A refusal or failure stops Run; it is
// never retried with a refreshed token.
func (m *tuiModel) runAnswered(msg responseMsg) tea.Cmd {
	m.pending = false
	seq := m.seq
	if msg.err != nil {
		m.seq = nil
		m.fresh, m.live = false, "reconnecting (daemon unavailable)"
		m.say("Run stopped: " + strings.ToLower(msg.label) + " was not confirmed: " + msg.err.Error() + ". Nothing else was sent.")
		return nil
	}
	if m.ordered(msg.v, true) {
		m.setView(msg.v, true)
	}
	if msg.v.Outcome != "ok" {
		m.seq = nil
		m.say("Run stopped: " + strings.ToLower(msg.label) + " refused: " + msg.v.Error + ". The screen shows the controller's current state; nothing was retried.")
		return m.tick()
	}
	if seq == nil {
		return m.tick()
	}
	if msg.v.Stream != m.v.Stream {
		m.seq = nil
		m.say("Run stopped after " + strings.ToLower(msg.label) + ": waiting for follower state after daemon restart. Press Run after resync.")
		return m.tick()
	}
	switch msg.op {
	case "apply":
		m.plan.agent = ""
	case "prepare":
		m.plan.target, m.plan.intent = "", ""
	case "review", "pipeline":
		m.seq = nil
		m.home(doneMessage(msg.op, msg.label, msg.v))
		return m.tick()
	}
	return tea.Batch(m.advance(), m.tick())
}

func doneMessage(op, label string, v Response) string {
	switch op {
	case "prepare":
		return "Intent and target saved. Nothing has run yet."
	case "resolve-rules":
		if v.RulesReady {
			return "Rules loaded."
		}
		return "Rules did not load; see the Rules step."
	case "review":
		return "Review started."
	case "pipeline":
		return "Pipeline started."
	case "handback":
		return "Fix handed back to your branch."
	case "apply":
		return "Draft applied to this run."
	case "save":
		return "Draft saved for new runs; this run did not change."
	case "cancel":
		return "Cancellation requested."
	case "new", "rerun-from-start":
		return "New run ready."
	}
	return label + ": done."
}

func (m *tuiModel) submit(label string, q Request) tea.Cmd {
	allowed := has(m.v.Actions, q.Op) || q.Op == "trust" && trustable(m.v) && !followOnly(m.v)
	switch {
	case m.pending:
		m.say("Still waiting for the previous action; nothing else was sent.")
		return nil
	case !m.fresh:
		m.say("Reconnecting: actions are disabled until fresh state arrives. Nothing was sent.")
		return nil
	case !allowed:
		m.say(label + " is not available in the current state. Nothing was sent.")
		return nil
	}
	m.pending = true
	m.message, m.failed = label+"…", false
	client := m.client
	return func() tea.Msg {
		v, err := client.send(q, false)
		return responseMsg{op: q.Op, label: label, v: v, err: err}
	}
}

func (m *tuiModel) simple(a tuiAction) tea.Cmd {
	return m.submit(a.Label, mutation(m.v, a.Op))
}

func (m *tuiModel) key(k tea.KeyMsg) tea.Cmd {
	key := k.String()
	if m.editing {
		return m.editKey(k)
	}
	if key == "q" || key == "ctrl+c" {
		m.farewell = "Detached. Nothing was cancelled. Run greenhouse here to reattach."
		if busy(m.v.Run) {
			m.farewell = "Detached. " + strings.ToUpper(progressLabel(m.v.Run)[:1]) + progressLabel(m.v.Run)[1:] + " keeps running in the Greenhouse daemon. Run greenhouse here to reattach."
		}
		return tea.Quit
	}
	if key == "?" && m.screen != screenPage {
		m.openPage("Help", "help", helpText(m))
		return nil
	}
	switch m.screen {
	case screenHome:
		return m.homeKey(key)
	case screenMenu:
		return m.menuKey(key)
	case screenSetup:
		return m.setupKey(key)
	case screenFindings:
		return m.findingsKey(key)
	case screenDecision:
		return m.decisionKey(key)
	case screenRecovery:
		return m.recoveryKey(key)
	case screenSettings:
		return m.settingsKey(key)
	case screenPage:
		return m.pageKey(key)
	case screenConfirm:
		return m.confirmKey(key)
	case screenIntent:
		return m.intentKey(key)
	}
	return nil
}

type homeTarget struct {
	label string
	run   func() tea.Cmd
	step  string
}

func homeTargets(m *tuiModel) []homeTarget {
	var out []homeTarget
	if a := primaryAction(m.v, m.agents); a.ID != "" {
		out = append(out, homeTarget{label: strings.ToLower(a.Label[:1]) + a.Label[1:], run: func() tea.Cmd { return m.start(a) }})
	} else {
		out = append(out, homeTarget{})
	}
	if r := m.v.Run; r != nil && has(m.v.Actions, "prepare") && len(m.agents) > 0 && !busy(r) && m.seq == nil {
		for i, name := range []string{"Agent", "Intent", "Target"} {
			step := i
			out = append(out, homeTarget{label: "edit " + strings.ToLower(name), step: name, run: func() tea.Cmd { m.openSetup(step); return nil }})
		}
	}
	if r := m.v.Run; r != nil && has(m.v.Actions, "apply") && !busy(r) && m.seq == nil {
		out = append(out, homeTarget{label: "edit rules", step: "Rules", run: func() tea.Cmd { return m.openSettings(7) }})
	}
	return out
}

func (m *tuiModel) homeKey(key string) tea.Cmd {
	targets := homeTargets(m)
	switch key {
	case "enter":
		if targets[m.focus].run == nil {
			m.say("Nothing to run from here now. Press a for all actions or ? for help.")
			return nil
		}
		return targets[m.focus].run()
	case "down", "tab", "j":
		m.focus = (m.focus + 1) % len(targets)
	case "up", "shift+tab", "k":
		m.focus = (m.focus + len(targets) - 1) % len(targets)
	case "esc":
		m.focus = 0
		m.stopRun()
	case "a":
		m.screen, m.cursor = screenMenu, 0
	case "s":
		return m.openSettings(0)
	case "f":
		m.openFindings()
	case "d":
		if m.v.Decision != nil && has(m.v.Actions, "decide") {
			m.openDecision()
		}
	case "i":
		m.openPage("Details", "details", "")
	case "o":
		return m.openLogs()
	case "x":
		if m.seq != nil {
			m.stopRun()
			return nil
		}
		if has(m.v.Actions, "cancel") {
			return m.start(labelled("cancel", m.v.Run))
		}
		m.say("Nothing is running that can be cancelled.")
	case "r":
		m.agents = installedAgents()
		client := m.client
		return func() tea.Msg {
			v, err := client.send(Request{Version: protocolMax, Op: "inspect"}, true)
			return liveUpdate{state: liveState(err), view: liveView(v, err)}
		}
	}
	return nil
}

func liveState(err error) string {
	if err != nil {
		return "reconnecting (daemon unavailable)"
	}
	return "live"
}

func liveView(v Response, err error) *Response {
	if err != nil || v.Outcome != "ok" {
		return nil
	}
	return &v
}

func (m *tuiModel) start(a tuiAction) tea.Cmd {
	switch a.ID {
	case "run":
		return m.runReview()
	case "setup":
		m.openSetup(0)
	case "fix-rules":
		return m.openSettings(7)
	case "findings", "repair":
		m.openFindings()
	case "decide":
		m.openDecision()
	case "recover":
		m.recovery.step, m.recovery.choice = 0, 0
		m.screen = screenRecovery
	case "settings":
		return m.openSettings(0)
	case "details":
		m.openPage("Details", "details", "")
	case "logs":
		return m.openLogs()
	case "help":
		m.openPage("Help", "help", helpText(m))
	case "cancel", "new", "rerun-from-start", "migrate", "trust":
		q := mutation(m.v, a.Op)
		switch a.Op {
		case "trust":
			q.Digest, q.Reason = m.v.Repository.Digest, "terminal trust"
		case "migrate":
			q.Reason = "terminal migrate"
		}
		m.ask(a, q)
	default:
		return m.simple(a)
	}
	return nil
}

func (m *tuiModel) ask(a tuiAction, q Request) {
	m.confirm, m.confirmQ, m.back = a, q, m.screen
	m.screen = screenConfirm
	m.input.SetValue("")
	if a.Confirm != "x" {
		m.edit("", "type "+a.Confirm)
	}
}

func (m *tuiModel) edit(value, placeholder string) {
	m.editing = true
	m.input.SetValue(value)
	m.input.Placeholder = placeholder
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *tuiModel) editKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc", "ctrl+c":
		m.editing = false
		m.input.Blur()
		return m.editDone(false)
	case "enter":
		return m.editDone(true)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return cmd
}

func (m *tuiModel) editDone(accept bool) tea.Cmd {
	value := strings.TrimSpace(m.input.Value())
	switch m.screen {
	case screenSetup:
		if !accept {
			m.setupValues[m.setupStep] = value
			m.setupStep--
			if m.setupStep == 0 {
				m.editing = false
			} else {
				m.edit(m.setupValues[m.setupStep], "")
			}
			return nil
		}
		if m.setupStep == 1 && len(value) > intentLimit {
			m.say(fmt.Sprintf("The intent is %d bytes; a review intent holds at most %d. Nothing was cut.", len(value), intentLimit))
			return nil
		}
		if value == "" && m.setupStep == 2 {
			m.say("Enter a Git target such as main, or press Esc to go back.")
			return nil
		}
		m.setupValues[m.setupStep] = value
		m.setupStep++
		if m.setupStep == 2 {
			m.edit(m.setupValues[2], "Git target such as main")
			return nil
		}
		m.editing = false
		m.input.Blur()
	case screenSettings:
		m.editing = false
		m.input.Blur()
		if accept {
			m.setField(m.cursor, value)
		}
	case screenDecision:
		return m.decisionText(accept, value)
	case screenIntent:
		return m.intentTyped(accept, value)
	case screenRecovery:
		if !accept {
			m.editing = false
			m.recovery.step = 0
			return nil
		}
		if !validText(value) {
			m.say("A reason is required; it is recorded with the recovery.")
			return nil
		}
		m.editing = false
		m.input.Blur()
		choice := m.v.Run.Recovery.Options[m.recovery.choice]
		q := mutation(m.v, "recover")
		q.Choice, q.Reason = choice, value
		a := tuiAction{ID: "recover", Op: "recover", Label: "Recovery " + choice, Hint: "Records the recovery choice with your reason.", Confirm: choice}
		m.ask(a, q)
		m.back = screenRecovery
	case screenConfirm:
		if !accept {
			m.editing = false
			m.screen = m.back
			if m.back == screenRecovery {
				m.recovery.step = 0
			}
			return nil
		}
		if value != m.confirm.Confirm {
			m.say("Type " + m.confirm.Confirm + " exactly to confirm, or press Esc. Nothing was sent.")
			return nil
		}
		m.editing = false
		m.input.Blur()
		return m.submit(m.confirm.Label, m.confirmQ)
	}
	return nil
}

func (m *tuiModel) menuKey(key string) tea.Cmd {
	items := menuItems(m.v, m.fresh && !m.pending)
	switch key {
	case "esc", "a":
		m.home("")
	case "down", "tab", "j":
		m.cursor = (m.cursor + 1) % len(items)
	case "up", "shift+tab", "k":
		m.cursor = (m.cursor + len(items) - 1) % len(items)
	case "enter":
		if m.cursor < len(items) {
			item := items[m.cursor]
			m.home("")
			return m.start(item)
		}
	}
	return nil
}

func (m *tuiModel) openSetup(step int) {
	r := m.v.Run
	m.screen, m.setupStep = screenSetup, 0
	m.agentPick = max(0, slices.Index(m.agents, m.planAgent()))
	m.setupValues = [3]string{"", intentBody(m.planIntent()), m.planTarget()}
	if m.plan.target == "" && r.Target != "" {
		m.setupValues[2] = r.Target
	}
	m.editing = false
	if step > 0 {
		m.setupStep = step
		m.edit(m.setupValues[step], "")
	}
}

func (m *tuiModel) setupKey(key string) tea.Cmd {
	switch m.setupStep {
	case 0:
		switch key {
		case "esc":
			m.home("")
		case "down", "tab", "j":
			m.agentPick = (m.agentPick + 1) % max(1, len(m.agents))
		case "up", "shift+tab", "k":
			m.agentPick = (m.agentPick + max(1, len(m.agents)) - 1) % max(1, len(m.agents))
		case "enter":
			if len(m.agents) == 0 {
				m.say("No supported agent is installed.")
				return nil
			}
			m.setupStep = 1
			m.edit(m.setupValues[1], "empty: collect from your agent session on Run")
		}
	case 3:
		switch key {
		case "esc":
			m.setupStep = 2
			m.edit(m.setupValues[2], "")
		case "enter":
			m.savePlan()
		}
	}
	return nil
}

func (m *tuiModel) savePlan() {
	agent := m.agents[m.agentPick]
	m.plan.agent, m.plan.target = agent, m.setupValues[2]
	if intent := m.setupValues[1]; intent != intentBody(m.planIntent()) {
		m.plan.intent = intent
	}
	m.home("Saved for Run. Nothing has run yet.")
}

func intentBody(intent string) string {
	if _, body, ok := strings.Cut(intent, "not a summary:\n\n"); ok {
		return strings.Join(strings.Fields(body), " ")
	}
	return intent
}

func (m *tuiModel) openFindings() {
	if m.v.Run == nil || len(m.v.Run.Findings) == 0 {
		m.say("There are no findings yet.")
		return
	}
	m.screen, m.cursor, m.expanded = screenFindings, 0, false
}

func selectable(r *Run) []string {
	if r == nil || r.Review == nil || r.Review.Result == nil {
		return nil
	}
	actionable, _ := outstanding(r)
	reviewed := findingIDs(r.Review.Result.Findings)
	var ids []string
	for _, f := range actionable {
		if has(reviewed, f.ID) {
			ids = append(ids, f.ID)
		}
	}
	return ids
}

func (m *tuiModel) findingsKey(key string) tea.Cmd {
	r := m.v.Run
	n := len(r.Findings)
	canRepair := has(m.v.Actions, "repair")
	switch key {
	case "esc":
		if m.expanded {
			m.expanded = false
			return nil
		}
		m.home("")
	case "down", "j", "tab":
		m.cursor = (m.cursor + 1) % n
	case "up", "k", "shift+tab":
		m.cursor = (m.cursor + n - 1) % n
	case "enter", "right", "l":
		m.expanded = !m.expanded
	case " ":
		id := r.Findings[m.cursor].ID
		switch {
		case !canRepair:
			m.say("Selected repair is not available now; findings can be read but not selected.")
		case !has(selectable(r), id):
			m.say("This finding cannot be repaired: it is informational, resolved or from an older head.")
		default:
			m.selected[id] = !m.selected[id]
		}
	case "r":
		if !canRepair {
			m.say("Selected repair is not available now.")
			return nil
		}
		var ids []string
		for _, f := range r.Findings {
			if m.selected[f.ID] && has(selectable(r), f.ID) {
				ids = append(ids, f.ID)
			}
		}
		if len(ids) == 0 {
			m.say("Select at least one finding with Space first. Nothing was sent.")
			return nil
		}
		q := mutation(m.v, "repair")
		q.Selected = ids
		a := labelled("repair", r)
		a.Label = "Repair " + plural(len(ids), "finding")
		a.Hint = fmt.Sprintf("The trusted fixer edits a private worktree, runs the trusted checks and commits; it uses one round (%d of %d used).", r.ReviewUsed+r.RepairUsed, r.Configs[len(r.Configs)-1].Settings.Limit)
		m.confirm, m.confirmQ, m.back = a, q, screenFindings
		m.screen = screenConfirm
	}
	return nil
}

func (m *tuiModel) openDecision() {
	m.screen, m.decision.step, m.decision.choice = screenDecision, 0, 0
	m.selected = map[string]bool{}
	for _, id := range m.v.Decision.Findings {
		m.selected[id] = true
	}
}

func (m *tuiModel) decisionChoice() string {
	return m.v.Decision.Options[m.decision.choice]
}

func (m *tuiModel) decisionKey(key string) tea.Cmd {
	d := m.v.Decision
	switch m.decision.step {
	case 0:
		switch key {
		case "esc":
			m.home("")
		case "down", "j", "tab":
			m.decision.choice = (m.decision.choice + 1) % len(d.Options)
		case "up", "k", "shift+tab":
			m.decision.choice = (m.decision.choice + len(d.Options) - 1) % len(d.Options)
		case "enter":
			switch {
			case m.decisionChoice() == "repair" && d.Kind == "findings":
				m.decision.step, m.cursor = 1, 0
			case m.decisionChoice() == "increase-cap":
				m.decision.step = 2
				m.edit("", "new limit, digits only")
			default:
				m.decision.step = 3
				m.edit("", "why this answer")
			}
		}
	case 1:
		switch key {
		case "esc":
			m.decision.step = 0
		case "down", "j", "tab":
			m.cursor = (m.cursor + 1) % len(d.Findings)
		case "up", "k", "shift+tab":
			m.cursor = (m.cursor + len(d.Findings) - 1) % len(d.Findings)
		case " ":
			id := d.Findings[m.cursor]
			m.selected[id] = !m.selected[id]
		case "enter":
			if m.groupSelection() == nil && !m.wholeGroup() {
				m.say("Select at least one finding with Space. Nothing was sent.")
				return nil
			}
			m.decision.step = 3
			m.edit("", "why this answer")
		}
	}
	return nil
}

func (m *tuiModel) wholeGroup() bool {
	for _, id := range m.v.Decision.Findings {
		if !m.selected[id] {
			return false
		}
	}
	return true
}

// groupSelection returns nil for the whole group, as the controller
// expects, and the chosen IDs otherwise.
func (m *tuiModel) groupSelection() []string {
	if m.wholeGroup() {
		return nil
	}
	var ids []string
	for _, id := range m.v.Decision.Findings {
		if m.selected[id] {
			ids = append(ids, id)
		}
	}
	return ids
}

func (m *tuiModel) decisionText(accept bool, value string) tea.Cmd {
	d := m.v.Decision
	if !accept {
		m.editing = false
		m.input.Blur()
		if m.decision.step == 3 && m.decisionChoice() == "repair" && d.Kind == "findings" {
			m.decision.step = 1
		} else {
			m.decision.step = 0
		}
		return nil
	}
	if m.decision.step == 2 {
		if _, err := parseCount(value); err != nil {
			m.say(err.Error() + "; nothing submitted")
			return nil
		}
		m.limitText = value
		m.decision.step = 3
		m.edit("", "why this answer")
		return nil
	}
	if !validText(value) {
		m.say("A reason is required; it is recorded with the decision.")
		return nil
	}
	m.editing = false
	m.input.Blur()
	q := mutation(m.v, "decide")
	q.Choice, q.Reason = m.decisionChoice(), value
	if q.Choice == "repair" && d.Kind == "findings" {
		q.Selected = m.groupSelection()
	}
	if q.Choice == "increase-cap" {
		q.Limit, _ = strconv.Atoi(m.limitText)
	}
	return m.submit("Decision "+q.Choice, q)
}

func (m *tuiModel) recoveryKey(key string) tea.Cmd {
	rec := m.v.Run.Recovery
	switch key {
	case "esc":
		m.home("")
	case "down", "j", "tab":
		m.recovery.choice = (m.recovery.choice + 1) % len(rec.Options)
	case "up", "k", "shift+tab":
		m.recovery.choice = (m.recovery.choice + len(rec.Options) - 1) % len(rec.Options)
	case "enter":
		m.recovery.step = 1
		m.edit("", "why this recovery")
	}
	return nil
}

var settingNames = []string{"Mode", "Preset", "Limit", "Reviewer agent", "Reviewer model", "Fixer agent", "Fixer model", "Rules", "Auto-resume"}
var settingKeys = []string{"mode", "preset", "limit", "reviewer_backend", "reviewer_model", "fixer_backend", "fixer_model", "rules", "auto_resume"}
var settingChoices = map[int][]string{0: {"auto", "ask", "yolo"}, 1: {"one-pass", "full"}, 3: {"pi", "claude", "codex"}, 5: {"pi", "claude", "codex"}, 8: {"", "on", "off"}}

type settingsTarget struct {
	op, target, label, scope string
}

func settingsTargets(v Response) []settingsTarget {
	var out []settingsTarget
	if has(v.Actions, "apply") {
		out = append(out, settingsTarget{"apply", "", "Apply to this run", "Adds a run-local snapshot. New runs keep their defaults."})
	}
	if has(v.Actions, "save") {
		out = append(out, settingsTarget{"save", "user", "Save as my default", "Changes per-user defaults for new runs. This run does not change."})
		out = append(out, settingsTarget{"save", "repository", "Save to repository override", "Writes .greenhouse.json for new runs; it applies once the repository is trusted. This run does not change."})
	}
	return out
}

func (m *tuiModel) current() Settings {
	r := m.v.Run
	return r.Configs[len(r.Configs)-1].Settings
}

func (m *tuiModel) openSettings(cursor int) tea.Cmd {
	m.screen, m.cursor, m.editing = screenSettings, cursor, false
	if m.draft == nil {
		draft := m.current()
		draft.Rules = slices.Clone(draft.Rules)
		m.draft = &draft
	}
	if m.models == nil && m.modelsErr == "" {
		client := m.client
		return func() tea.Msg {
			v, err := client.send(Request{Version: protocolMax, Op: "models"}, false)
			switch {
			case err != nil:
				return modelsMsg{err: err.Error()}
			case v.Outcome != "ok":
				return modelsMsg{err: v.Error}
			}
			return modelsMsg{models: v.Models}
		}
	}
	return nil
}

func fieldValue(s Settings, i int) string {
	switch i {
	case 0:
		return s.Mode
	case 1:
		return s.Preset
	case 2:
		return strconv.Itoa(s.Limit)
	case 3:
		return s.ReviewerBackend
	case 4:
		return s.ReviewerModel
	case 5:
		return s.FixerBackend
	case 6:
		return s.FixerModel
	case 7:
		return strings.Join(s.Rules, ", ")
	}
	return s.AutoResume
}

func (m *tuiModel) setField(i int, value string) {
	d := m.draft
	switch i {
	case 0:
		d.Mode = value
	case 1:
		d.Preset = value
	case 2:
		n, err := parseCount(value)
		if err != nil {
			m.say("Limit: " + err.Error() + ". The draft keeps " + strconv.Itoa(d.Limit) + ".")
			return
		}
		d.Limit = n
	case 3:
		if d.ReviewerBackend != value {
			d.ReviewerModel = ""
		}
		d.ReviewerBackend = value
	case 4:
		d.ReviewerModel = value
	case 5:
		if d.FixerBackend != value {
			d.FixerModel = ""
		}
		d.FixerBackend = value
	case 6:
		d.FixerModel = value
	case 7:
		d.Rules = []string{}
		for _, rule := range strings.Split(value, ",") {
			if name := strings.TrimSpace(rule); name != "" {
				d.Rules = append(d.Rules, name)
			}
		}
	case 8:
		d.AutoResume = value
	}
}

func (m *tuiModel) settingsKey(key string) tea.Cmd {
	targets := settingsTargets(m.v)
	rows := len(settingNames) + len(targets) + 1
	switch key {
	case "esc":
		m.home("")
		if m.draftChanged() {
			m.message = "Settings draft kept; it is not applied or saved."
		}
	case "down", "j", "tab":
		m.cursor = (m.cursor + 1) % rows
	case "up", "k", "shift+tab":
		m.cursor = (m.cursor + rows - 1) % rows
	case "left", "right", " ", "h", "l":
		choices, ok := settingChoices[m.cursor]
		if !ok {
			return nil
		}
		step := 1
		if key == "left" || key == "h" {
			step = len(choices) - 1
		}
		at := slices.Index(choices, fieldValue(*m.draft, m.cursor))
		m.setField(m.cursor, choices[(at+step)%len(choices)])
	case "enter":
		switch {
		case m.cursor < len(settingNames):
			if _, ok := settingChoices[m.cursor]; ok {
				return m.settingsKey("right")
			}
			m.edit(fieldValue(*m.draft, m.cursor), settingPlaceholder(m.cursor))
		case m.cursor < len(settingNames)+len(targets):
			t := targets[m.cursor-len(settingNames)]
			q := mutation(m.v, t.op)
			draft := *m.draft
			q.Settings, q.Reason, q.Target = &draft, "terminal Settings", t.target
			return m.submit(t.label, q)
		default:
			draft := m.current()
			draft.Rules = slices.Clone(draft.Rules)
			m.draft = &draft
			m.message, m.failed = "Draft reset to the run's current settings.", false
		}
	}
	return nil
}

func settingPlaceholder(i int) string {
	switch i {
	case 2:
		return "combined review and repair rounds"
	case 4, 6:
		return "empty uses the agent's native default"
	case 7:
		return "comma-separated; ?name is optional; file:/absolute/path"
	}
	return ""
}

func (m *tuiModel) draftChanged() bool {
	return m.draft != nil && digest(*m.draft) != digest(m.current())
}

func (m *tuiModel) openLogs() tea.Cmd {
	client, run := m.client, m.v.RunID
	return func() tea.Msg {
		v, err := client.send(Request{Version: protocolMax, Op: "transcript", RunID: run}, false)
		return transcriptMsg{v, err}
	}
}

func (m *tuiModel) openPage(title, kind, text string) {
	if m.screen != screenPage {
		m.back = m.screen
	}
	m.screen, m.pageTitle, m.pageKind, m.pageText, m.scroll = screenPage, title, kind, text, 0
}

func (m *tuiModel) pageKey(key string) tea.Cmd {
	switch key {
	case "esc", "enter":
		m.screen = m.back
		if m.back == screenPage || m.back == "" {
			m.screen = screenHome
		}
	case "down", "j":
		m.scroll++
	case "up", "k":
		m.scroll = max(0, m.scroll-1)
	case "pgdown", " ":
		m.scroll += max(1, m.height-4)
	case "pgup":
		m.scroll = max(0, m.scroll-max(1, m.height-4))
	case "home", "g":
		m.scroll = 0
	case "c":
		if m.pageKind == "details" {
			m.criteria = !m.criteria
		}
	case "o":
		if m.pageKind == "details" {
			return m.openLogs()
		}
	}
	return nil
}

func (m *tuiModel) confirmKey(key string) tea.Cmd {
	switch {
	case key == "esc":
		m.screen = m.back
	case m.confirm.Confirm == "x" && key == "x":
		return m.submit(m.confirm.Label, m.confirmQ)
	case m.confirm.Confirm == "" && key == "enter":
		return m.submit(m.confirm.Label, m.confirmQ)
	}
	return nil
}

func helpText(m *tuiModel) string {
	var b strings.Builder
	b.WriteString("Keys\n")
	b.WriteString("  Enter      run the highlighted action\n  ↑ ↓ Tab    move between choices\n  Esc        go back one step\n  a          all actions available now\n  s          settings draft (Apply or Save explicitly)\n  f          findings\n  d          answer the open decision\n  i          details: IDs, digests, sources, rules\n  o          transcript log\n  x          cancel running work (press x twice)\n  r          recheck agents and reconnect\n  ?          this help\n  q, Ctrl-C  detach; work keeps running in the daemon\n")
	b.WriteString("\nTyping inside a text field never detaches or cancels; Esc leaves the field.\n")
	b.WriteString("\nActions\n")
	available := map[string]bool{}
	for _, item := range menuItems(m.v, m.fresh) {
		available[item.ID] = true
	}
	for _, id := range []string{"run", "setup", "resolve-rules", "review", "pipeline", "decide", "resolve-conflict", "repair", "handback", "rerun", "cancel-auto-resume", "cancel", "recover", "migrate", "trust", "rerun-from-start", "new"} {
		a := labelled(id, m.v.Run)
		state := "not available now"
		if available[id] {
			state = "available"
		}
		fmt.Fprintf(&b, "  %s: %s\n    %s\n", a.Label, state, a.Hint)
	}
	b.WriteString("\nThe daemon decides which actions are available. AXI (greenhouse axi) exposes the same state as JSON.\n")
	return b.String()
}

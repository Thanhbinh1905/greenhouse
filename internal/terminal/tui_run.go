package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
	sessionintent "github.com/Thanhbinh1905/greenhouse/internal/intent"
	"github.com/Thanhbinh1905/greenhouse/internal/repository"
	tea "github.com/charmbracelet/bubbletea"
)

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
	match sessionintent.Match
	err   error
}

type collector func(ctx context.Context, worktree, target, head string) (sessionintent.Match, error)

func inferCommitIntent(worktree, target, head string) (string, string, error) {
	branch, err := repository.Read(worktree, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", "", errors.New("the current branch has no name")
	}
	branchName := strings.TrimSpace(string(branch))
	if body, source, ok := pullRequestBody(worktree, branchName); ok {
		return body, source, nil
	}
	base, err := repository.CommittedTarget(worktree, target)
	if err != nil {
		return "", "", err
	}
	messages, err := repository.Read(worktree, "log", "--format=%s", "--end-of-options", base+".."+head, "--")
	if err != nil {
		return "", "", errors.New("cannot read commit messages for this branch")
	}
	lines := strings.FieldsFunc(strings.TrimSpace(string(messages)), func(r rune) bool { return r == '\n' || r == '\r' })
	if len(lines) == 0 {
		return "", "", errors.New("no committed changes exist between the target and this branch")
	}
	source := "commit messages + branch " + branchName
	return strings.Join(lines, "; "), source, nil
}

func pullRequestBody(worktree, branch string) (string, string, bool) {
	remote, err := repository.Read(worktree, "remote", "get-url", "origin")
	if err != nil {
		return "", "", false
	}
	owner, repo, ok := githubRepository(strings.TrimSpace(string(remote)))
	if !ok {
		return "", "", false
	}
	if gh, err := exec.LookPath("gh"); err == nil {
		if body, ok := ghPullRequestBody(worktree, gh); ok {
			return body, "open pull request body", true
		}
	}
	token := firstEnvironment("GH_TOKEN", "GITHUB_TOKEN")
	if token == "" {
		return "", "", false
	}
	base := strings.TrimRight(os.Getenv("GREENHOUSE_GITHUB_API"), "/")
	if base == "" {
		base = "https://api.github.com"
	}
	endpoint := base + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/pulls?state=open&per_page=100&head=" + url.QueryEscape(owner+":"+branch)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", false
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", "", false
	}
	var pulls []struct {
		Number int    `json:"number"`
		Body   string `json:"body"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&pulls) != nil || len(pulls) != 1 || pulls[0].Number < 1 || strings.TrimSpace(pulls[0].Body) == "" || len(pulls[0].Body) > sessionintent.Limit {
		return "", "", false
	}
	return strings.TrimSpace(pulls[0].Body), fmt.Sprintf("PR #%d body", pulls[0].Number), true
}

func ghPullRequestBody(worktree, gh string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, gh, "pr", "view", "--json", "body")
	cmd.Dir = worktree
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", false
	}
	if err := cmd.Start(); err != nil {
		return "", false
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, (1<<20)+1))
	waitErr := cmd.Wait()
	if readErr != nil || waitErr != nil || len(data) > 1<<20 {
		return "", false
	}
	var result struct {
		Body string `json:"body"`
	}
	if json.Unmarshal(data, &result) != nil {
		return "", false
	}
	body := strings.TrimSpace(result.Body)
	if body == "" || len(body) > sessionintent.Limit {
		return "", false
	}
	return body, true
}

func githubRepository(remote string) (string, string, bool) {
	remote = strings.TrimSuffix(remote, ".git")
	if strings.HasPrefix(remote, "git@github.com:") {
		remote = strings.TrimPrefix(remote, "git@github.com:")
	} else {
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Host != "github.com" || parsed.User != nil {
			return "", "", false
		}
		remote = strings.TrimPrefix(parsed.Path, "/")
	}
	parts := strings.Split(remote, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func firstEnvironment(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func localCollector(ctx context.Context, worktree, target, head string) (sessionintent.Match, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return sessionintent.Match{}, err
	}
	resolved, err := repository.CommittedTarget(worktree, target)
	if err != nil {
		return sessionintent.Match{}, errors.New("the target " + target + " is not a commit here; choose another target")
	}
	return sessionintent.Collect(ctx, home, worktree, resolved, head)
}

var runStepLabels = map[string]string{
	"collect":       "Collecting intent from local agent sessions",
	"apply":         "Selecting the agent",
	"prepare":       "Saving intent and target",
	"resolve-rules": "Loading rules",
	"review":        "Starting the review",
	"pipeline":      "Starting the pipeline",
}

func (m *tuiModel) planAgent() string {
	r := m.v.Run
	if m.plan.agent != "" {
		return m.plan.agent
	}
	if initialRunPending(m.v) && len(m.agents) > 0 {
		return m.agents[0]
	}
	switch {
	case r.Reviewer != nil:
		return engine.BackendOf(*r.Reviewer)
	}
	if backend := m.current().ReviewerBackend; slices.Contains(m.agents, backend) {
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
	m.seq = &runSeq{intent: m.planIntent(), source: m.intentSource, sent: map[string]bool{}}
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
	allowed := func(op string) bool { return slices.Contains(m.v.Actions, op) }
	switch {
	case seq.intent == "":
		return m.collect(target)
	case r.Reviewer == nil && (agent != settings.ReviewerBackend || initialRunPending(m.v) && settings.ReviewerModel != ""):
		draft := settings
		draft.ReviewerBackend, draft.ReviewerModel = agent, ""
		q := engine.TerminalMutation(m.v, "apply")
		q.Settings, q.Reason = &draft, "select installed agent"
		return m.step("apply", q)
	case r.Reviewer == nil || r.Intent != seq.intent || r.Target == "" || m.plan.target != "" && !sameTarget(r, m.plan.target):
		q := engine.TerminalMutation(m.v, "prepare")
		q.Intent, q.IntentSource, q.Target = seq.intent, seq.source, target
		return m.step("prepare", q)
	case !m.v.RulesReady:
		return m.step("resolve-rules", engine.TerminalMutation(m.v, "resolve-rules"))
	case !full:
		return m.step("review", engine.TerminalMutation(m.v, "review"))
	case allowed("pipeline"):
		return m.step("pipeline", engine.TerminalMutation(m.v, "pipeline"))
	}
	m.seq = nil
	m.say("Run cannot start the pipeline in the current state. Nothing else was sent.")
	return nil
}

func sameTarget(r *engine.Run, target string) bool {
	resolved, err := repository.CommittedTarget(r.Submitted.Worktree, target)
	return err == nil && resolved == r.Target
}

func (m *tuiModel) step(op string, q engine.Request) tea.Cmd {
	m.seq.step = op
	if m.seq.sent[op] {
		m.seq = nil
		m.say("Run stopped: " + strings.ToLower(runStepLabels[op]) + " did not change the run state. Nothing else was sent.")
		return nil
	}
	m.seq.sent[op] = true
	if !slices.Contains(m.v.Actions, op) {
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
		return m.useSession(msg.match.Candidates[0])
	default:
		intent, source, err := m.fallback(m.v.Run.Submitted.Worktree, m.planTarget(), m.v.Run.Candidate)
		if err != nil {
			m.seq = nil
			m.home("No committed change to review. Commit your changes on this branch, then run greenhouse again.")
			return nil
		}
		m.seq.intent, m.seq.source, m.plan.intent, m.intentSource = intent, source, intent, source
		m.home("")
		return m.advance()
	}
}

func (m *tuiModel) useSession(s *sessionintent.Session) tea.Cmd {
	intent, ok := s.Intent()
	if !ok {
		fallback, source, err := m.fallback(m.v.Run.Submitted.Worktree, m.planTarget(), m.v.Run.Candidate)
		if err != nil {
			m.seq = nil
			m.home("The agent session intent is too long and no shorter committed intent was available. Edit the intent before running greenhouse again.")
			return nil
		}
		m.seq.intent, m.seq.source, m.plan.intent, m.intentSource = fallback, source, fallback, source
		m.home("")
		return m.advance()
	}
	m.seq.intent, m.seq.source, m.plan.intent, m.intentSource = intent, s.Label(), intent, s.Label()
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

func (m *tuiModel) intentLabel() (string, string) {
	intent := m.planIntent()
	switch {
	case intent == "":
		return stepTodo, "inferring from your agent session"
	case strings.HasPrefix(intent, sessionintent.CollectedPrefix):
		state := stepTodo
		if m.v.Run.Intent == intent {
			state = stepDone
		}
		return state, intentSummary(intent)
	case m.v.Run.Intent == intent:
		return stepDone, intent
	}
	if m.intentSource != "" {
		return stepTodo, intent
	}
	return stepTodo, intent + " · typed"
}

func storedIntentSource(intent string) string {
	if strings.HasPrefix(intent, sessionintent.CollectedPrefix) {
		source, _, _ := strings.Cut(strings.TrimPrefix(intent, sessionintent.CollectedPrefix), ". These are the user's own words from that session, not a summary:")
		return strings.TrimSpace(source)
	}
	if strings.HasPrefix(intent, "PR #") {
		if source, _, ok := strings.Cut(intent, ": "); ok {
			return source
		}
	}
	if strings.HasPrefix(intent, "Commit messages and branch ") {
		if branch, _, ok := strings.Cut(strings.TrimPrefix(intent, "Commit messages and branch "), ": "); ok {
			return "commit messages + branch " + branch
		}
	}
	return ""
}

func intentSummary(intent string) string {
	if !strings.HasPrefix(intent, sessionintent.CollectedPrefix) {
		return intent
	}
	source, body, _ := strings.Cut(strings.TrimPrefix(intent, sessionintent.CollectedPrefix), ". These are the user's own words from that session, not a summary:\n\n")
	line, _, _ := strings.Cut(body, "\n")
	return "“" + line + "” · collected from " + source
}

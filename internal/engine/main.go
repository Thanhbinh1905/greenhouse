package engine

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"
)

type Settings struct {
	Mode            string   `json:"mode"`
	Preset          string   `json:"preset"`
	Limit           int      `json:"limit"`
	Rules           []string `json:"rules"`
	ReviewerBackend string   `json:"reviewer_backend"`
	ReviewerModel   string   `json:"reviewer_model"`
	FixerBackend    string   `json:"fixer_backend"`
	FixerModel      string   `json:"fixer_model"`
	AutoResume      string   `json:"auto_resume,omitempty"`
}
type Identity struct {
	Repository string `json:"repository"`
	Worktree   string `json:"worktree"`
	Branch     string `json:"branch"`
	Head       string `json:"head"`
}
type Snapshot struct {
	ID           string            `json:"id"`
	Settings     Settings          `json:"settings"`
	Source       string            `json:"source"`
	LegacyDigest string            `json:"legacy_digest,omitempty"`
	Sources      map[string]string `json:"sources,omitempty"`
}
type Event struct {
	Kind   string `json:"kind"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	Used   int    `json:"used"`
	Before int    `json:"before"`
	After  int    `json:"after"`
	Review int    `json:"review,omitempty"`
	Repair int    `json:"repair,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
}
type Run struct {
	Intent        string          `json:"intent,omitempty"`
	Reviewer      *ReviewerConfig `json:"reviewer,omitempty"`
	SelectedDiff  string          `json:"selected_diff,omitempty"`
	Findings      []Finding       `json:"findings,omitempty"`
	Review        *Execution      `json:"review,omitempty"`
	RuleSnapshots []RuleSnapshot  `json:"rule_snapshots,omitempty"`
	ID            string          `json:"id"`
	Input         string          `json:"input"`
	Token         string          `json:"token"`
	Submitted     Identity        `json:"submitted"`
	Target        string          `json:"target"`
	Candidate     string          `json:"candidate"`
	Stage         string          `json:"stage"`
	Status        string          `json:"status"`
	Configs       []Snapshot      `json:"configs"`
	ReviewUsed    int             `json:"review_used"`
	RepairUsed    int             `json:"repair_used"`
	Repair        *Repair         `json:"repair,omitempty"`
	Pipeline      *Pipeline       `json:"pipeline,omitempty"`
	Events        []Event         `json:"events"`
	Predecessor   string          `json:"predecessor,omitempty"`
	Successor     string          `json:"successor,omitempty"`
	Pause         *BackendPause   `json:"pause,omitempty"`
	Attempts      []Attempt       `json:"attempts,omitempty"`
	AutoRetries   int             `json:"auto_retries,omitempty"`
	PriorRepairs  []Repair        `json:"prior_repairs,omitempty"`
	Recovery      *Recovery       `json:"recovery,omitempty"`
	Recoveries    []Recovery      `json:"recoveries,omitempty"`
	Inherited     *Inheritance    `json:"inherited,omitempty"`
	Host          *Host           `json:"host,omitempty"`
	HostGrants    []string        `json:"host_grants,omitempty"`

	ReviewBase       string `json:"review_base,omitempty"`
	ReviewBaseReason string `json:"review_base_reason,omitempty"`
}
type Request struct {
	Version  int       `json:"version"`
	Op       string    `json:"op"`
	Path     string    `json:"path"`
	RunID    string    `json:"run_id"`
	Expected string    `json:"expected"`
	Input    string    `json:"input_id"`
	Gate     string    `json:"gate_id"`
	Action   string    `json:"action_id"`
	Settings *Settings `json:"settings,omitempty"`
	Intent   string    `json:"intent,omitempty"`
	Target   string    `json:"target,omitempty"`
	Reason   string    `json:"reason"`
	Selected []string  `json:"selected,omitempty"`
	Choice   string    `json:"choice,omitempty"`
	Limit    int       `json:"limit,omitempty"`
	Client   string    `json:"client,omitempty"`
	Digest   string    `json:"digest,omitempty"`
	After    uint64    `json:"after,omitempty"`
	Stream   string    `json:"stream,omitempty"`
	WaitMS   int       `json:"wait_ms,omitempty"`
	Host     *Host     `json:"host,omitempty"`
	Grant    string    `json:"grant,omitempty"`
}
type Response struct {
	Models      []string          `json:"models,omitempty"`
	RuleScope   string            `json:"rule_scope,omitempty"`
	Rules       []RuleEvidence    `json:"rules,omitempty"`
	RulesReady  bool              `json:"rules_ready"`
	Version     int               `json:"version"`
	Outcome     string            `json:"outcome"`
	Error       string            `json:"error,omitempty"`
	RunID       string            `json:"run_id,omitempty"`
	Input       string            `json:"input_id,omitempty"`
	Gate        string            `json:"gate_id,omitempty"`
	Token       string            `json:"token,omitempty"`
	Run         *Run              `json:"run,omitempty"`
	Defaults    Settings          `json:"defaults"`
	Blocked     []string          `json:"blocked"`
	Actions     []string          `json:"actions"`
	Decision    *Decision         `json:"decision,omitempty"`
	Budget      *Budget           `json:"budget,omitempty"`
	Explanation string            `json:"explanation,omitempty"`
	Config      []ConfigStatus    `json:"config"`
	Migrations  []Migration       `json:"migrations,omitempty"`
	Sources     map[string]string `json:"defaults_sources,omitempty"`
	Repository  *RepositoryStatus `json:"repository,omitempty"`
	Transcript  *TranscriptView   `json:"transcript,omitempty"`
	Protocol    *Protocol         `json:"protocol,omitempty"`
	Stream      string            `json:"stream,omitempty"`
	Sequence    uint64            `json:"sequence,omitempty"`
	Events      []FeedEvent       `json:"events,omitempty"`
	Wait        string            `json:"wait,omitempty"`
	HostResult  *HostResult       `json:"host_result,omitempty"`
	replayed    bool
}
type Receipt struct {
	Digest string   `json:"digest"`
	Result Response `json:"result"`
}
type Store struct {
	Version        int                `json:"version"`
	Defaults       Settings           `json:"defaults"`
	DefaultsSource string             `json:"defaults_source"`
	Runs           map[string]*Run    `json:"runs"`
	Slots          map[string]string  `json:"slots"`
	Receipts       map[string]Receipt `json:"receipts"`
	Migrations     []Migration        `json:"migrations,omitempty"`
	Trust          map[string]Trust   `json:"trust"`
}

func opaque() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func legacySnapshotDigest(s Snapshot) string {
	type settings struct {
		Mode   string   `json:"mode"`
		Preset string   `json:"preset"`
		Limit  int      `json:"limit"`
		Rules  []string `json:"rules"`
	}
	return digest(struct {
		ID       string   `json:"id"`
		Settings settings `json:"settings"`
		Source   string   `json:"source"`
	}{s.ID, settings{s.Settings.Mode, s.Settings.Preset, s.Settings.Limit, s.Settings.Rules}, s.Source})
}
func snapshotDigestMatches(s Snapshot, expected string) bool {
	return expected == digest(s) || (s.LegacyDigest != "" && s.LegacyDigest == legacySnapshotDigest(s) && expected == s.LegacyDigest)
}
func reviewerConfigDigest(config ReviewerConfig) string {
	if config.Provider == (ProviderConfig{}) && config.Backend == "" {
		return digest(struct {
			Executable string `json:"executable"`
			Model      string `json:"model"`
		}{config.Executable, config.Model})
	}
	return digest(config)
}
func resolve(path string) (Identity, error) {
	run := func(args ...string) (string, error) {
		c := exec.Command("git", append([]string{"-C", path}, args...)...)
		for _, variable := range os.Environ() {
			if !strings.HasPrefix(variable, "GIT_") {
				c.Env = append(c.Env, variable)
			}
		}
		c.Env = append(c.Env, "GIT_OPTIONAL_LOCKS=0")
		b, e := c.Output()
		return strings.TrimSpace(string(b)), e
	}
	root, e := run("rev-parse", "--show-toplevel")
	if e != nil {
		return Identity{}, errors.New("a Git worktree with a commit is required")
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return Identity{}, e
	}
	common, e := run("rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return Identity{}, e
	}
	common, e = filepath.EvalSymlinks(common)
	if e != nil {
		return Identity{}, e
	}
	head, e := run("rev-parse", "--verify", "HEAD")
	if e != nil {
		return Identity{}, e
	}
	branch, _ := run("symbolic-ref", "-q", "HEAD")
	return Identity{common, root, branch, head}, nil
}
func home() (string, error) {
	h := os.Getenv("GREENHOUSE_HOME")
	if h == "" {
		u, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		h = filepath.Join(u, ".local", "state", "greenhouse")
	}
	h, e := filepath.Abs(h)
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(h, 0700); e != nil {
		return "", e
	}
	info, e := os.Stat(h)
	if e != nil {
		return "", e
	}
	if info.Mode().Perm()&0077 != 0 {
		return "", errors.New("state home must be private (chmod 700)")
	}
	return h, nil
}
func persist(h string, s *Store) error {
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(h, "state-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(f.Name(), filepath.Join(h, "state.json")); e != nil {
		return e
	}
	d, e := os.Open(h)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func load(h string) (*Store, error) {
	s := &Store{Version: 1, Defaults: Settings{Mode: "auto", Preset: "one-pass", Limit: 4, Rules: []string{"unslop", "no-comments"}, ReviewerBackend: defaultAgent(), FixerBackend: defaultAgent()}, DefaultsSource: "built-in default", Runs: map[string]*Run{}, Slots: map[string]string{}, Receipts: map[string]Receipt{}, Trust: map[string]Trust{}}
	b, e := os.ReadFile(filepath.Join(h, "state.json"))
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(b, s); e != nil {
		return nil, e
	}
	initial := Settings{ReviewerBackend: "pi", ReviewerModel: "openai/gpt-5.6-luna", FixerBackend: "pi", FixerModel: "openai/gpt-5.6-luna"}
	if s.Trust == nil {
		s.Trust = map[string]Trust{}
	}
	if s.DefaultsSource == "" {
		s.DefaultsSource = "built-in default"
	}
	s.Defaults = withModelDefaults(s.Defaults, initial)
	for _, run := range s.Runs {
		if run != nil {
			for i := range run.Configs {
				snapshot := &run.Configs[i]
				settings := snapshot.Settings
				if settings.ReviewerBackend == "" && settings.ReviewerModel == "" && settings.FixerBackend == "" && settings.FixerModel == "" && snapshot.LegacyDigest == "" {
					snapshot.LegacyDigest = legacySnapshotDigest(*snapshot)
				}
				snapshot.Settings = withModelDefaults(settings, initial)
			}
		}
	}
	if s.Version != 1 || s.Runs == nil || s.Slots == nil || s.Receipts == nil {
		return nil, errors.New("unsupported or incomplete store")
	}
	for key, r := range s.Runs {
		if r == nil || key != r.ID || r.Input == "" || r.Token == "" || r.Candidate == "" || len(r.Configs) == 0 || r.ReviewUsed < 0 || r.RepairUsed < 0 {
			return nil, errors.New("invalid durable run")
		}
		if r.Review != nil {
			x := r.Review
			if r.ReviewUsed < 1 && x.Status != "paused" || x.ID == "" || x.Backend != executionBackends[backendOf(x.Config)] || x.InputDigest != bytesDigest(x.Input) || x.ConfigDigest != reviewerConfigDigest(x.Config) || x.Rules.Digest != bytesDigest(x.Rules.Content) || (x.Rules.Candidate != r.Candidate && r.Pipeline == nil && (r.Repair == nil || x.Rules.Candidate != r.Repair.Base)) || !reviewArgsMatch(x) {
				return nil, errors.New("invalid durable review evidence")
			}
		}
		if err := validateRuleSnapshots(r); err != nil {
			return nil, err
		}
		for _, snapshot := range r.Configs {
			if snapshot.ID == "" || !valid(snapshot.Settings) {
				return nil, errors.New("invalid configuration snapshot")
			}
		}
		if p := r.Pipeline; p != nil && p.Status == "locally-ready" && !localOnlyPipeline(p) {
			p.Status, p.Failed, p.Reason = "needs-decision", "push", "delivery stages require revalidation"
			r.Status, r.Stage = "needs-decision", "push"
			for _, name := range []string{"push", "pr", "ci"} {
				if stage := p.stage(name); stage == nil {
					p.Stages = append(p.Stages, StageResult{Name: name, Status: "pending", Reason: "delivery evidence must be collected"})
				} else if stage.Status == "unsupported" {
					stage.Status, stage.Reason = "pending", "delivery evidence must be collected"
				}
			}
		}
		if r.Status == "running" || r.Status == "cancelling" || r.Status == "writing" {
			if p := r.Pipeline; p != nil && p.Active != nil && p.Active.Kind == "delivery" && r.Status == "running" {
				st := p.stage(p.Active.Stage)
				if st == nil {
					return nil, errors.New("interrupted delivery stage is missing")
				}
				st.Status, st.Inputs, st.Reason = "pending", "", "interrupted delivery reconciled from durable stage identity"
				p.Active, p.Status = nil, "running"
				r.Status, r.Stage, r.Token = "running", "pipeline", opaque()
				r.Events = append(r.Events, Event{Kind: "delivery-reconcile", Actor: "daemon", Reason: "resume idempotent forge step without agent dispatch"})
				continue
			}
			if p := r.Pipeline; p != nil {
				kind, stage, execution := "candidate", r.Stage, ""
				if p.Active != nil {
					kind, stage, execution = p.Active.Kind, p.Active.Stage, p.Active.ID
					if p.Active.Kind == "review" && r.Review != nil {
						r.Review.Status = "unknown-outcome"
					}
					if (p.Active.Kind == "repair" || p.Active.Kind == "conflict") && len(p.Repairs) > 0 {
						p.Repairs[len(p.Repairs)-1].Status = "unknown-outcome"
						p.Repairs[len(p.Repairs)-1].Writer = "unknown"
					}
				}
				holdRecovery(r, kind, stage, execution, "controller stopped during a pipeline step; no retry")
			} else if r.Repair != nil && r.Repair.Status != "fixed-not-independently-rereviewed" {
				r.Repair.Status = "unknown-outcome"
				r.Repair.Writer = "unknown"
				requireRecovery(r, "repair", "selected-repair", r.Repair.ID, "controller stopped during the selected repair; no retry")
			} else if r.Review != nil {
				r.Review.Status = "unknown-outcome"
				requireRecovery(r, "review", "independent-review", r.Review.ID, "controller stopped during the independent review; no retry")
			} else {
				requireRecovery(r, "run", r.Stage, "", "controller stopped while the run was "+r.Status+"; no retry")
			}
			r.Events = append(r.Events, Event{Kind: "interrupted", Actor: "daemon", Reason: "unknown execution outcome; no retry"})
		}
	}
	for _, id := range s.Slots {
		if s.Runs[id] == nil {
			return nil, errors.New("invalid active run slot")
		}
	}
	if !valid(s.Defaults) {
		return nil, errors.New("invalid defaults")
	}
	return s, nil
}
func view(s *Store, r *Run) Response {
	v := Response{Version: 1, Outcome: "ok", Defaults: s.Defaults, Blocked: []string{}, Actions: []string{"attach", "inspect"}, Config: configStatuses(), Migrations: s.Migrations, Sources: userSources(s)}
	for _, c := range v.Config {
		if c.Status == "older" || c.Status == "newer" || c.Status == "invalid" {
			v.Blocked = append(v.Blocked, "configuration: "+c.Error)
		}
		if c.Status == "older" && !has(v.Actions, "migrate") {
			v.Actions = append(v.Actions, "migrate")
		}
	}
	if r != nil {
		var status RepositoryStatus
		v.Defaults, v.Sources, status = repositoryDefaults(s, r.Submitted)
		v.Repository = &status
		v.RuleScope = effectiveRuleScope(r)
		v.Rules, v.RulesReady = effectiveRules(r)
		if !v.RulesReady {
			v.Blocked = append(v.Blocked, "required rules unresolved or failed; use resolve-rules and inspect errors")
		}
		if r.Intent == "" || r.Target == "" || r.Reviewer == nil {
			v.Blocked = append(v.Blocked, "prepare requires intent, a Git target and an installed authenticated agent")
		}
		v.Run = r
		v.Budget = budgetOf(r)
		v.RunID = r.ID
		v.Input = r.Input
		v.Token = r.Token
		v.Gate = currentGate(r)
		id, e := resolve(r.Submitted.Worktree)
		safe := e == nil && id.Repository == r.Submitted.Repository && id.Worktree == r.Submitted.Worktree && r.Status != "running" && r.Status != "cancelling" && r.Status != "writing" && r.Status != "recovery-required"
		held := custodyHeld(s, r)
		if held {
			v.Blocked = append(v.Blocked, "another run holds unpublished or ambiguous repository candidate custody; writer actions blocked")
		}
		if safe {
			if id == r.Submitted {
				v.Actions = append(v.Actions, "apply", "save", "resolve-rules")
				if !held && r.Status == "outstanding-findings" && r.Repair == nil && r.RepairUsed == 0 && r.ReviewUsed+r.RepairUsed < r.Configs[len(r.Configs)-1].Settings.Limit && repairable(r) {
					v.Actions = append(v.Actions, "repair")
				}
				if r.Repair != nil && r.Repair.Status == "fixed-not-independently-rereviewed" && (r.Repair.Handback == "awaiting-handback" || r.Repair.Handback == "handback-in-progress") {
					v.Actions = append(v.Actions, "handback")
				}
				if p := r.Pipeline; !held && p != nil && r.Configs[len(r.Configs)-1].Settings.Preset == "full" && p.Status != "ci-ready" && p.Status != "locally-ready" && !released(p) && p.open() == nil {
					v.Actions = append(v.Actions, "pipeline")
				}
				if p := r.Pipeline; !held && p != nil && p.open() != nil {
					v.Actions = append(v.Actions, "decide")
				}
				if p := r.Pipeline; !held && p != nil && p.Gate != "" {
					v.Actions = append(v.Actions, "resolve-conflict")
				}
				if p := r.Pipeline; p != nil && (p.Status == "ci-ready" || p.Status == "locally-ready" && localOnlyPipeline(p)) && r.Candidate != r.Submitted.Head && p.Handback != "handed-back" {
					v.Actions = append(v.Actions, "handback")
				}
				if ownReviews(r) == 0 && (r.Pipeline == nil || r.Pipeline.Worktree == "") {
					v.Actions = append(v.Actions, "prepare")
					if r.Intent != "" && r.Target != "" && r.Reviewer != nil && v.RulesReady && r.Configs[len(r.Configs)-1].Settings.Preset == "one-pass" {
						v.Actions = append(v.Actions, "review")
					}
				}
			}
			v.Actions = append(v.Actions, "new")
		}
		if e != nil || id != r.Submitted {
			if r.Status == "clean-review" || r.Status == "outstanding-findings" {
				snapshot := *r
				snapshot.Status = "stale"
				v.Run = &snapshot
			}
			v.Blocked = append(v.Blocked, "source head or branch moved; old-run advancement blocked")
		}
		if r.Status == "running" || r.Status == "writing" && r.Repair != nil && (r.Repair.Status == "fixing" || r.Repair.Status == "checking" || r.Repair.Status == "creating-candidate" || r.Repair.Status == "running") {
			v.Actions = append(v.Actions, "cancel")
		}
		if r.Pause != nil {
			v.Explanation = pauseSentence(r)
			v.Blocked = append(v.Blocked, "paused: backend "+r.Pause.Reason)
			if safe && id == r.Submitted {
				v.Actions = append(v.Actions, "resume", "rerun")
				if r.Pause.AutoResumeAt != "" {
					v.Actions = append(v.Actions, "cancel-auto-resume")
				}
			}
		} else if safe && id == r.Submitted && rerunnable(r) {
			v.Actions = append(v.Actions, "rerun")
		}
		if safe && id == r.Submitted && r.Successor == "" && (r.Repair == nil || r.Repair.Handback == "handed-back") && (r.Pause != nil || rerunnable(r)) {
			v.Actions = append(v.Actions, "rerun-from-start")
		}
		if r.Host == nil && r.ReviewUsed+r.RepairUsed >= r.Configs[len(r.Configs)-1].Settings.Limit {
			v.Blocked = append(v.Blocked, "combined review and repair budget exhausted")
		}
		if r.Host == nil && r.Status == "outstanding-findings" && r.Repair == nil && !repairable(r) {
			v.Blocked = append(v.Blocked, "selected repair unavailable: actionable findings and private trusted fixer/check configuration are required")
		}
		if r.RepairUsed > 0 && r.Pipeline == nil {
			v.Blocked = append(v.Blocked, "optional repair admission consumed; no second repair")
		}
		if r.Repair != nil && r.Repair.Status == "fixed-not-independently-rereviewed" {
			v.Blocked = append(v.Blocked, "fixed but not independently rereviewed")
		}
		if r.Review != nil && r.Review.Status == "clean-review" && (e != nil || id != r.Submitted) {
			v.Blocked = append(v.Blocked, "historical clean review only; source moved")
		}
		if rec := r.Recovery; rec != nil {
			v.Blocked = append(v.Blocked, "unknown "+rec.Kind+" outcome at "+rec.Stage+"; explicit recovery: "+strings.Join(rec.Options, ", "))
			v.Actions = append(v.Actions, "recover")
		}
		if p := r.Pipeline; p != nil && p.Failed != "" && p.Status != "running" && p.open() == nil {
			v.Blocked = append(v.Blocked, "pipeline stopped at "+p.Failed+": "+p.Reason)
		}
		if p := r.Pipeline; p != nil && p.open() != nil {
			v.Decision = p.open()
			v.Blocked = append(v.Blocked, "decision required: "+v.Decision.Kind+" ("+strings.Join(v.Decision.Options, ", ")+")")
		}
		if r.Host != nil {
			hostView(&v, r, e == nil && id == r.Submitted, safe && id == r.Submitted)
		}
	}
	return v
}
func backendOf(config ReviewerConfig) string {
	if config.Backend == "" {
		return backendPi
	}
	return config.Backend
}
func defaultAgent() string {
	agents := installedAgents()
	if len(agents) > 0 {
		return agents[0]
	}
	return backendPi
}
func validModelSelection(backend, model string) bool {
	if model == "" {
		_, ok := executionBackends[backend]
		return ok
	}
	if backend == backendClaude || backend == backendCodex {
		if len(model) > 128 || model[0] == '-' || model[0] == '.' {
			return false
		}
		for _, c := range model {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
				return false
			}
		}
		return true
	}
	provider, modelID, qualified := strings.Cut(model, "/")
	return backend == "pi" && validText(model) && len(model) <= 256 && qualified && provider != "" && modelID != "" && !strings.ContainsAny(model, "*?[] \t\r\n:") && !strings.HasPrefix(model, "-")
}
func currentGate(r *Run) string {
	if r != nil && r.Recovery != nil {
		return r.Recovery.ID
	}
	if r != nil && r.Pipeline != nil && r.Pipeline.Gate != "" {
		return r.Pipeline.Gate
	}
	return "none"
}
func pipelineCustody(r *Run) bool {
	p := r.Pipeline
	return p != nil && p.Worktree != "" && !released(p) && r.Candidate != r.Submitted.Head && p.Handback != "handed-back"
}
func valid(s Settings) bool {
	return (s.Mode == "auto" || s.Mode == "ask" || s.Mode == "yolo") && (s.Preset == "one-pass" || s.Preset == "full") && s.Limit >= 1 && s.Limit <= 1000 && len(s.Rules) <= 32 && validModelSelection(s.ReviewerBackend, s.ReviewerModel) && validModelSelection(s.FixerBackend, s.FixerModel) && (s.AutoResume == "" || s.AutoResume == "on" || s.AutoResume == "off")
}
func withModelDefaults(s Settings, defaults Settings) Settings {
	if s.ReviewerBackend == "" {
		s.ReviewerBackend = defaults.ReviewerBackend
		if s.ReviewerModel == "" {
			s.ReviewerModel = defaults.ReviewerModel
		}
	}
	if s.FixerBackend == "" {
		s.FixerBackend = defaults.FixerBackend
		if s.FixerModel == "" {
			s.FixerModel = defaults.FixerModel
		}
	}
	return s
}
func handle(s *Store, q Request) Response {
	v := handleRequest(s, q)
	if v.Run != nil && v.Run.Host != nil && !sameHost(v.Run.Host, q.Host) {
		v.Actions = []string{"attach", "inspect"}
	}
	return v
}
func handleRequest(s *Store, q Request) Response {
	fail := func(msg string, r *Run) Response { v := view(s, r); v.Outcome = "refused"; v.Error = msg; return v }
	if e := checkVersion(q); e != nil {
		return fail(e.Error(), nil)
	}
	id, e := resolve(q.Path)
	if e != nil {
		return fail(e.Error(), nil)
	}
	if q.Op == "models" {
		config, err := trustedReviewer()
		if err != nil {
			return fail(err.Error(), nil)
		}
		models, err := nativeModelCatalog(config.Executable)
		if err != nil {
			return fail(err.Error(), nil)
		}
		response := Response{Version: 1, Outcome: "ok", Models: make([]string, 0, len(models))}
		for model := range models {
			response.Models = append(response.Models, model)
		}
		sort.Strings(response.Models)
		return response
	}
	slot := digest([]string{id.Repository, id.Worktree})
	r := s.Runs[s.Slots[slot]]
	if q.Op == "inspect" && q.RunID != "" {
		other := s.Runs[q.RunID]
		if other == nil || other.Submitted.Repository != id.Repository {
			return fail("unknown run for this repository", r)
		}
		v := view(s, other)
		if other != r {
			v.Actions = []string{"attach", "inspect"}
		}
		return v
	}
	if q.Op == "transcript" {
		target := r
		if q.RunID != "" {
			target = s.Runs[q.RunID]
		}
		if target == nil || target.Submitted.Repository != id.Repository {
			return fail("unknown run for this repository", r)
		}
		transcript, err := readTranscript(target.ID)
		if err != nil {
			return fail(err.Error(), r)
		}
		v := view(s, r)
		v.Transcript = transcript
		return v
	}
	if q.Op == "attach" || q.Op == "inspect" {
		if q.Op == "attach" && q.Host != nil {
			if refusal := hostRefusal(r, q); refusal != "" {
				return fail(refusal, r)
			}
		}
		if r == nil && q.Op == "attach" {
			r = &Run{ID: opaque(), Input: opaque(), Token: opaque(), Submitted: id, Candidate: id.Head, Stage: "intent", Status: "unconfigured", Configs: []Snapshot{initialSnapshot(s, id)}, Events: []Event{{Kind: "created", Actor: "client"}}, Host: q.Host}
			s.Runs[r.ID] = r
			s.Slots[slot] = r.ID
		}
		return view(s, r)
	}
	if q.Op != "apply" && q.Op != "save" && q.Op != "new" && q.Op != "resolve-rules" && q.Op != "review" && q.Op != "prepare" && q.Op != "cancel" && q.Op != "repair" && q.Op != "handback" && q.Op != "pipeline" && q.Op != "resolve-conflict" && q.Op != "decide" && q.Op != "resume" && q.Op != "rerun" && q.Op != "rerun-from-start" && q.Op != "cancel-auto-resume" && q.Op != "migrate" && q.Op != "trust" && q.Op != "recover" {
		return fail("unsupported operation: "+q.Op, r)
	}
	if q.Action == "" {
		return fail("action_id required", r)
	}
	if refusal := hostRefusal(r, q); refusal != "" {
		return fail(refusal, r)
	}
	d := digest(q)
	if receipt, ok := s.Receipts[q.Action]; ok {
		if receipt.Digest != d {
			return fail("action_id reused with different input", r)
		}
		replay := receipt.Result
		replay.replayed = true
		return replay
	}
	actor := "axi"
	if q.Client == "terminal" {
		actor = "terminal"
	}
	if q.Op == "trust" {
		_, _, status := repositoryDefaults(s, id)
		if !validText(q.Reason) {
			return fail("reason required", r)
		}
		if q.Digest != status.Digest {
			return fail("override file changed or was not inspected; inspect it again before trusting", r)
		}
		s.Trust[id.Repository] = Trust{Repository: id.Repository, Path: status.Path, Digest: status.Digest, Actor: actor, Reason: q.Reason, Action: q.Action}
		v := view(s, r)
		s.Receipts[q.Action] = Receipt{d, frozen(v)}
		return v
	}
	if q.Op == "migrate" {
		if !validText(q.Reason) {
			return fail("reason required", r)
		}
		records, err := migrateConfig(actor, q.Reason, q.Action)
		s.Migrations = append(s.Migrations, records...)
		if err != nil {
			return fail(err.Error(), r)
		}
		v := view(s, r)
		v.Migrations = records
		s.Receipts[q.Action] = Receipt{d, frozen(v)}
		return v
	}
	if r == nil || q.RunID != r.ID || q.Expected != r.Token || q.Input != r.Input || q.Gate != currentGate(r) {
		return fail("stale run/input/gate/token", r)
	}
	if q.Op == "new" && (repairHeld(r.Repair) || pipelineCustody(r)) {
		return fail("repair custody remains unpublished or ambiguous; explicit handback required", r)
	}
	handbackHead := r.Candidate
	if r.Repair != nil {
		handbackHead = r.Repair.Head
	}
	handbackReconcile := q.Op == "handback" && (r.Repair != nil || r.Pipeline != nil) && id.Repository == r.Submitted.Repository && id.Worktree == r.Submitted.Worktree && id.Branch == r.Submitted.Branch && id.Head == handbackHead
	if q.Op != "new" && q.Op != "cancel" && q.Op != "recover" && id != r.Submitted && !handbackReconcile {
		return fail("source head or branch moved", r)
	}
	if q.Op == "cancel" && (r.Status == "running" || r.Status == "writing" && r.Repair != nil && (r.Repair.Status == "fixing" || r.Repair.Status == "checking" || r.Repair.Status == "creating-candidate" || r.Repair.Status == "running")) {
		r.Status = "cancelling"
		r.Token = opaque()
	} else if q.Op != "recover" && (r.Status == "running" || r.Status == "cancelling" || r.Status == "writing" || r.Status == "recovery-required") {
		return fail("unsafe replacement or mutation; explicit recovery required", r)
	}
	paused := r
	if q.Op == "cancel" {
		if r.Status != "cancelling" {
			return fail("no live execution", r)
		}
	} else if q.Op == "recover" {
		next, err := recoverRun(s, r, id, Answer{Choice: q.Choice, Actor: actor, Reason: q.Reason, Action: q.Action})
		if err != nil {
			return fail(err.Error(), r)
		}
		if next != r {
			s.Slots[slot] = next.ID
		}
		r = next
	} else if q.Op == "cancel-auto-resume" {
		if r.Pause == nil {
			return fail("no backend pause", r)
		}
		r.Token = opaque()
	} else if q.Op == "resume" || q.Op == "rerun" && r.Pause != nil {
		if err := resumeRun(s, r, actor); err != nil {
			return fail(err.Error(), r)
		}
	} else if q.Op == "rerun" {
		if err := rerunStage(s, r, actor); err != nil {
			return fail(err.Error(), r)
		}
		r.AutoRetries = 0
		r.Token = opaque()
	} else if q.Op == "prepare" {
		if ownReviews(r) != 0 || !validText(q.Intent) || r.Pipeline != nil && r.Pipeline.Worktree != "" {
			return fail("immutable review intent required before dispatch", r)
		}
		if err := prepareRun(r, q.Intent, q.Target); err != nil {
			return fail(err.Error(), r)
		}
		r.Token = opaque()
	} else if q.Op == "rerun-from-start" {
		if repairHeld(r.Repair) {
			return fail("repair custody remains unpublished or ambiguous; explicit handback required", r)
		}
		r = restartRun(s, r, id, actor)
		s.Slots[slot] = r.ID
	} else if q.Op == "review" && r.Host != nil {
		if err := admitHostReview(r, q.Grant); err != nil {
			return fail(err.Error(), r)
		}
	} else if q.Op == "review" {
		if err := admitReview(r); err != nil {
			return fail(err.Error(), r)
		}
	} else if q.Op == "repair" {
		if err := admitRepair(s, r, q); err != nil {
			return fail(err.Error(), r)
		}
	} else if q.Op == "pipeline" {
		if err := admitPipeline(s, r); err != nil {
			return fail(err.Error(), r)
		}
		r.Token = opaque()
	} else if q.Op == "resolve-conflict" {
		if custodyHeld(s, r) {
			return fail("repository candidate custody is already held", r)
		}
		if err := admitConflictRepair(r, q); err != nil {
			return fail(err.Error(), r)
		}
		r.Events = append(r.Events, Event{Kind: "resolve-conflict", Actor: "client", Reason: "authorized fixer for target conflict"})
		r.Token = opaque()
	} else if q.Op == "decide" {
		var d *Decision
		if r.Pipeline != nil {
			d = r.Pipeline.open()
		}
		if d == nil || d.ID != q.Gate {
			return fail("no open decision for this gate", r)
		}
		if custodyHeld(s, r) {
			return fail("repository candidate custody is already held", r)
		}
		actor := q.Client
		if actor != "terminal" {
			actor = "axi"
		}
		next, err := applyChoice(s, r, d, Answer{Choice: q.Choice, Actor: actor, Reason: q.Reason, Action: q.Action, Selected: q.Selected, Limit: q.Limit})
		if err != nil {
			return fail(err.Error(), r)
		}
		r = next
	} else if q.Op == "handback" {
		authorize := authorizeHandback
		if r.Repair == nil && r.Pipeline != nil {
			authorize = authorizePipelineHandback
		}
		if err := authorize(r); err != nil {
			return fail(err.Error(), r)
		}
		r.Token = opaque()
	} else if q.Op == "resolve-rules" {
		if r.Repair != nil {
			return fail("repair inputs are immutable after authorization", r)
		}
		if r.Review != nil && r.Pause == nil {
			r.Status = "stale"
		}
		r.RuleSnapshots = append(r.RuleSnapshots, resolveRules(r))
		invalidate(r, "")
		supersede(r)
		r.Token = opaque()
	} else if q.Op == "apply" || q.Op == "save" {
		if q.Settings != nil {
			normalized := withModelDefaults(*q.Settings, r.Configs[len(r.Configs)-1].Settings)
			q.Settings = &normalized
		}
		if q.Settings == nil || !valid(*q.Settings) {
			return fail("invalid settings", r)
		}
		if q.Settings.Limit < r.ReviewUsed+r.RepairUsed {
			return fail("limit below consumed usage", r)
		}
		if q.Reason == "" {
			return fail("reason required", r)
		}
		if q.Op == "apply" {
			if r.Repair != nil && r.Pause == nil {
				return fail("repair inputs and authority are immutable after authorization", r)
			}
			if r.Review != nil && r.Pause == nil {
				r.Status = "stale"
			}
			old := r.Configs[len(r.Configs)-1].Settings
			if r.Reviewer != nil && (old.ReviewerModel != q.Settings.ReviewerModel || old.ReviewerBackend != q.Settings.ReviewerBackend) {
				cfg, err := trustedAgent(q.Settings.ReviewerBackend, q.Settings.ReviewerModel)
				if err != nil && q.Settings.ReviewerBackend != backendPi {
					return fail(err.Error(), r)
				}
				if err != nil {
					cfg = *r.Reviewer
					cfg.Model = q.Settings.ReviewerModel
				}
				r.Reviewer = &cfg
				if r.Review != nil && r.Review.Status != "paused" {
					r.Review.Status = "stale"
				}
			}
			r.Configs = append(r.Configs, nextSnapshot(r.Configs[len(r.Configs)-1], *q.Settings, sourceRunLocal))
			for _, role := range []struct{ name, fromBackend, fromModel, toBackend, toModel string }{{"reviewer", old.ReviewerBackend, old.ReviewerModel, q.Settings.ReviewerBackend, q.Settings.ReviewerModel}, {"fixer", old.FixerBackend, old.FixerModel, q.Settings.FixerBackend, q.Settings.FixerModel}} {
				if role.fromBackend != role.toBackend || role.fromModel != role.toModel {
					r.Events = append(r.Events, Event{Kind: "backend-switch", Actor: actor, Reason: role.name + ": " + q.Reason, From: role.fromBackend + " " + role.fromModel, To: role.toBackend + " " + role.toModel})
				}
			}
			invalidate(r, "")
			supersede(r)
			r.Events = append(r.Events, Event{Kind: q.Op, Actor: "client", Reason: q.Reason, Used: r.ReviewUsed + r.RepairUsed, Before: old.Limit, After: q.Settings.Limit, Review: r.ReviewUsed, Repair: r.RepairUsed})
		} else if q.Target == "repository" {
			if err := saveRepositoryOverride(s, id, *q.Settings, actor, q.Reason, q.Action); err != nil {
				return fail(err.Error(), r)
			}
			r.Events = append(r.Events, Event{Kind: "save", Actor: actor, Reason: "repository override: " + q.Reason})
		} else if q.Target == "" || q.Target == "user" {
			s.Defaults = *q.Settings
			s.DefaultsSource = "saved"
			r.Events = append(r.Events, Event{Kind: "save", Actor: "client", Reason: q.Reason})
		} else {
			return fail("save target must be user or repository", r)
		}
		r.Token = opaque()
	} else {
		prior := r.ID
		findings := append([]Finding(nil), r.Findings...)
		next := &Run{Findings: findings, ID: opaque(), Input: opaque(), Token: opaque(), Submitted: id, Candidate: id.Head, Stage: "intent", Status: "unconfigured", Configs: []Snapshot{initialSnapshot(s, id)}, Events: []Event{{Kind: "new", Actor: "client", Reason: "replaces " + prior}}}
		if q.Host != nil {
			hostSuccessor(r, next, q.Host)
		}
		r = next
		s.Runs[r.ID] = r
		s.Slots[slot] = r.ID
	}
	cancelAutoResume(paused, actor)
	v := view(s, r)
	b, _ := json.Marshal(v)
	var frozen Response
	json.Unmarshal(b, &frozen)
	s.Receipts[q.Action] = Receipt{d, frozen}
	return v
}
func frozen(v Response) Response {
	b, _ := json.Marshal(v)
	var out Response
	json.Unmarshal(b, &out)
	return out
}
func acceptRepairProgress(r *Run, id string, value Repair) bool {
	if r == nil || r.Repair == nil || r.Repair.ID != id || r.Status == "cancelling" && value.Status == "committing" {
		return false
	}
	r.Repair = &value
	return true
}

func daemon(h string) error {
	f, e := os.OpenFile(filepath.Join(h, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return errors.New("daemon already owns state home")
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	s, e := load(h)
	if e != nil {
		return e
	}
	if e = persist(h, s); e != nil {
		return e
	}
	socket := filepath.Join(h, "socket")
	if c, e := net.DialTimeout("unix", socket, time.Second); e == nil {
		c.Close()
		return errors.New("live listener exists; refusing replacement")
	}
	if e = os.Remove(socket); e != nil && !os.IsNotExist(e) {
		return e
	}
	l, e := net.Listen("unix", socket)
	if e != nil {
		return e
	}
	defer l.Close()
	pid, _ := json.Marshal(os.Getpid())
	if e = os.WriteFile(filepath.Join(h, "pid"), pid, 0600); e != nil {
		return e
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	go func() { <-signals; l.Close() }()
	type completion struct {
		run, execution, status, log string
		result                      *Verdict
		repair                      *Repair
		failure                     *BackendFailure
		answer                      agentAnswer
		snapshot                    string
		confinement                 *Confinement
	}
	type repairProgress struct {
		run, id string
		value   Repair
		ack     chan bool
	}
	completed := make(chan completion, 16)
	repairCompleted := make(chan completion)
	repairProgresses := make(chan repairProgress)
	connections := make(chan net.Conn)
	stopped := make(chan struct{})
	defer close(stopped)
	active := map[string]context.CancelFunc{}
	pipelineDone := make(chan stepOutcome)
	pipelineProgress := make(chan repairProgress)
	reconcileHandbacks(s)
	reconcileRecoveries(s)
	releaseDeadLeases(s)
	if err := persist(h, s); err != nil {
		return err
	}
	defer func() {
		for _, cancel := range active {
			cancel()
		}
	}()
	events := newFeed(s)
	defer events.close()
	checkpoint := func(runID, id string) func(*Repair) bool {
		return func(value *Repair) bool {
			ack := make(chan bool, 1)
			select {
			case pipelineProgress <- repairProgress{run: runID, id: id, value: *value, ack: ack}:
			case <-stopped:
				return false
			}
			select {
			case ok := <-ack:
				return ok
			case <-stopped:
				return false
			}
		}
	}
	schedule := func() error {
		for _, r := range s.Runs {
			p := r.Pipeline
			if p == nil || r.Status != "running" || active[r.ID] != nil {
				continue
			}
			if p.Active == nil {
				advancePipeline(s, r)
			}
			step := p.Active
			if step != nil && step.Kind == "sync" {
				if err := withWriterLock(r.Submitted.Repository, func() error { return ensureCandidate(r) }); err != nil {
					p.Active = nil
					if errors.Is(err, errCandidateUnknown) {
						holdRecovery(r, "candidate", step.Stage, step.ID, err.Error())
					} else {
						park(r, p.stage(step.Stage), "needs-decision", "private candidate unavailable: "+err.Error())
					}
					step = nil
				}
			}
			if err := persist(h, s); err != nil {
				return err
			}
			if step == nil {
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			active[r.ID] = cancel
			o := stepOutcome{Run: r.ID, Step: step.ID}
			ctx = withExecutor(ctx, step.ID)
			pc, target, candidate, source := *p, r.Target, r.Candidate, r.Submitted
			var review Execution
			if r.Review != nil {
				review = *r.Review
			}
			var repair Repair
			if len(p.Repairs) > 0 {
				repair = p.Repairs[len(p.Repairs)-1]
			}
			go func() {
				defer cancel()
				switch step.Kind {
				case "sync":
					o.Stage, o.Head, o.Conflict = syncTarget(ctx, pc, target, candidate)
				case "commands":
					o.Stage = runStageCommands(ctx, pc, step.Stage, candidate)
				case "delivery":
					o.Stage, o.Head, o.URL = runDeliveryStage(ctx, pc, step.Stage, candidate)
					o.Status, o.Log = o.Stage.Status, o.Stage.Reason
				case "review":
					reviewed := executeReview(ctx, &review)
					o.Status, o.Result, o.Log, o.Failure, o.Answer, o.Snapshot, o.Confinement = reviewed.Status, reviewed.Result, reviewed.Log, reviewed.Failure, reviewed.Answer, reviewed.Snapshot, reviewed.Confinement
				case "repair":
					executeRepair(ctx, &repair, source, checkpoint(o.Run, repair.ID))
					o.Repair = &repair
				case "conflict":
					executeConflictRepair(ctx, &repair, pc, target, source, checkpoint(o.Run, repair.ID))
					o.Repair = &repair
				}
				select {
				case pipelineDone <- o:
				case <-stopped:
				}
			}()
		}
		return nil
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				close(connections)
				return
			}
			select {
			case connections <- c:
			case <-stopped:
				c.Close()
				return
			}
		}
	}()
	launch := func() error {
		for _, r := range s.Runs {
			if r.Status == "cancelling" {
				if cancel := active[r.ID]; cancel != nil {
					cancel()
				}
				continue
			}
			if r.Status == "writing" && r.Repair != nil && r.Repair.Status == "admitted" {
				x := *r.Repair
				r.Repair.Status = "running"
				if err := persist(h, s); err != nil {
					return err
				}
				ctx, cancel := context.WithCancel(context.Background())
				active[r.ID] = cancel
				ctx = withExecutor(ctx, x.ID)
				go func(runID string, repair Repair, source Identity, ctx context.Context, cancel context.CancelFunc) {
					defer cancel()
					executeRepair(ctx, &repair, source, func(value *Repair) bool {
						ack := make(chan bool, 1)
						select {
						case repairProgresses <- repairProgress{run: runID, id: repair.ID, value: *value, ack: ack}:
						case <-stopped:
							return false
						}
						select {
						case ok := <-ack:
							return ok
						case <-stopped:
							return false
						}
					})
					select {
					case repairCompleted <- completion{run: runID, execution: repair.ID, repair: &repair}:
					case <-stopped:
					}
				}(r.ID, x, r.Submitted, ctx, cancel)
				continue
			}
			if r.Status != "running" || r.Review == nil || active[r.ID] != nil || r.Pipeline != nil {
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			active[r.ID] = cancel
			runID, x := r.ID, *r.Review
			go func() {
				reviewed := executeReview(ctx, &x)
				cancel()
				select {
				case completed <- completion{run: runID, execution: x.ID, status: reviewed.Status, log: reviewed.Log, result: reviewed.Result, failure: reviewed.Failure, answer: reviewed.Answer, snapshot: reviewed.Snapshot, confinement: reviewed.Confinement}:
				case <-stopped:
				}
			}()
		}
		return schedule()
	}
	wake := time.NewTimer(time.Hour)
	defer wake.Stop()
	orphanProbe := time.NewTicker(250 * time.Millisecond)
	defer orphanProbe.Stop()
	subscribers := time.NewTimer(time.Hour)
	defer subscribers.Stop()
	for {
		events.observe(s)
		events.flush(s, time.Now())
		subscribers.Reset(events.next(time.Now()))
		wake.Reset(untilAutoResume(s, time.Now()))
		var probe <-chan time.Time
		if needsProbe(s) {
			probe = orphanProbe.C
		}
		var c net.Conn
		select {
		case <-subscribers.C:
			continue
		case <-probe:
			if reconcileRecoveries(s) {
				if err := persist(h, s); err != nil {
					return err
				}
			}
			continue
		case <-wake.C:
			if fireAutoResumes(s, time.Now()) {
				if err := persist(h, s); err != nil {
					return err
				}
				if err := launch(); err != nil {
					return err
				}
			}
			continue
		case progress := <-pipelineProgress:
			ok := acceptPipelineProgress(s.Runs[progress.run], progress.id, progress.value)
			if ok {
				if err := persist(h, s); err != nil {
					progress.ack <- false
					return err
				}
			}
			progress.ack <- ok
			continue
		case o := <-pipelineDone:
			delete(active, o.Run)
			r := s.Runs[o.Run]
			if r == nil {
				return errors.New("pipeline run identity mismatch")
			}
			if err := completeStep(r, o); err != nil {
				return err
			}
			if err := persist(h, s); err != nil {
				return err
			}
			if err := schedule(); err != nil {
				return err
			}
			continue
		case progress := <-repairProgresses:
			r := s.Runs[progress.run]
			ok := acceptRepairProgress(r, progress.id, progress.value)
			if ok {
				if err := persist(h, s); err != nil {
					progress.ack <- false
					return err
				}
			}
			progress.ack <- ok
			continue
		case done := <-repairCompleted:
			select {
			case completed <- done:
			case <-stopped:
			}
			continue
		case done := <-completed:
			r := s.Runs[done.run]
			if r == nil {
				return errors.New("execution identity mismatch")
			}
			if done.repair != nil {
				delete(active, r.ID)
				if r.Repair == nil || r.Repair.ID != done.execution {
					return errors.New("repair identity mismatch")
				}
				r.Repair = done.repair
				r.Status = done.repair.Status
				if done.repair.Head != "" {
					r.Candidate = done.repair.Head
				}
				if r.Status == "unknown-outcome" {
					requireRecovery(r, "repair", "selected-repair", done.repair.ID, "repair writer outcome unknown: "+done.repair.Log)
				}
				x := r.Repair
				recordAttempt(r, Attempt{Execution: x.ID, Stage: "selected-repair", Kind: "repair", Role: "fixer", Backend: x.Config.Backend, Model: x.Config.Model, Outcome: x.Status, Reason: failureReason(x.Failure), Message: failureMessage(x.Failure), CostUSD: x.CostUSD, Tokens: x.Tokens, Thinking: x.Thinking, Partial: x.Partial})
				if x.Status == "paused" {
					refundRepair(r, x)
					pauseRun(r, "selected-repair", "repair", "fixer", fixerAgent(x.Config), x.ID, x.Failure, time.Now())
				}
				r.Token = opaque()
				if err := persist(h, s); err != nil {
					return err
				}
				continue
			}
			if r.Review == nil || r.Review.ID != done.execution {
				return errors.New("execution identity mismatch")
			}
			delete(active, r.ID)
			recordReviewEvidence(r.Review, done.snapshot, done.confinement)
			finishReview(r, reviewOutcome{Status: done.status, Log: done.log, Result: done.result, Failure: done.failure, Answer: done.answer})
			if err := persist(h, s); err != nil {
				return err
			}
			continue
		case conn, ok := <-connections:
			if !ok {
				return nil
			}
			c = conn
		}
		c.SetDeadline(time.Now().Add(5 * time.Second))
		var q Request
		dec := json.NewDecoder(io.LimitReader(c, 65536))
		dec.DisallowUnknownFields()
		e = dec.Decode(&q)
		var v Response
		committed := events.seq
		if e == nil && q.Op == "watch" {
			var parked bool
			if v, parked = events.subscribe(s, c, q); parked {
				continue
			}
		} else if e != nil {
			v = Response{Version: 1, Outcome: "refused", Error: "invalid request: " + e.Error()}
		} else {
			b, _ := json.Marshal(s)
			var next Store
			json.Unmarshal(b, &next)
			v = handle(&next, q)
			if e = persist(h, &next); e != nil {
				v = Response{Version: 1, Outcome: "error", Error: "storage commit failed: " + e.Error()}
				json.NewEncoder(c).Encode(v)
				c.Close()
				return e
			}
			s = &next
			reconcileHandbacks(s)
			releaseDeadLeases(s)
			if q.Op == "handback" && v.Outcome == "ok" {
				r := s.Runs[q.RunID]
				v = view(s, r)
				if r != nil && r.Repair != nil && r.Repair.Handback == "awaiting-handback" {
					v.Outcome = "refused"
					v.Error = r.Repair.Log
				}
				if r != nil && r.Repair == nil && r.Pipeline != nil && (r.Pipeline.Handback == "awaiting-handback" || r.Pipeline.Handback == "unknown-outcome") {
					v.Outcome = "refused"
					v.Error = r.Pipeline.HandbackError
				}
				b, _ := json.Marshal(v)
				var frozen Response
				json.Unmarshal(b, &frozen)
				s.Receipts[q.Action] = Receipt{Digest: digest(q), Result: frozen}
			}
			if q.Version >= 2 && !v.replayed {
				v.Stream, v.Sequence = events.stream, committed
				if receipt, ok := s.Receipts[q.Action]; ok && receipt.Digest == digest(q) {
					receipt.Result.Stream, receipt.Result.Sequence = v.Stream, v.Sequence
					s.Receipts[q.Action] = receipt
				}
			}
			if e = persist(h, s); e != nil {
				return e
			}
			if e = launch(); e != nil {
				return e
			}
		}
		stamp(&v, q.Version)
		json.NewEncoder(c).Encode(v)
		c.Close()
	}
}
func call(h string, q Request) (Response, error) {
	return request(h, q, true)
}
func request(h string, q Request, spawn bool) (Response, error) {
	var v Response
	socket := filepath.Join(h, "socket")
	c, e := net.DialTimeout("unix", socket, time.Second)
	if e != nil && spawn {
		exe, x := os.Executable()
		if x != nil {
			return v, x
		}
		cmd := exec.Command(exe, "daemon")
		cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+h)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if x = cmd.Start(); x != nil {
			return v, x
		}
		go cmd.Wait()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			c, e = net.DialTimeout("unix", socket, 100*time.Millisecond)
			if e == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if e != nil {
		return v, fmt.Errorf("daemon unavailable: %w", e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10*time.Second + time.Duration(q.WaitMS)*time.Millisecond))
	if e = json.NewEncoder(c).Encode(q); e != nil {
		return v, e
	}
	e = json.NewDecoder(c).Decode(&v)
	return v, e
}
func terminalWidth() int {
	size := [4]uint16{}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size))); err == 0 && size[1] > 1 {
		return int(size[1]) - 1
	}
	return 80
}
func wrapWords(text string, width int) string {
	indent := text[:len(text)-len(strings.TrimLeft(text, " "))]
	if len(indent) >= width-1 {
		indent = ""
	}
	available := width - len(indent)
	var lines []string
	line := indent
	count := 0
	rest := strings.TrimLeft(text, " ")
	for _, word := range strings.Fields(text) {
		at := strings.Index(rest, word)
		gap := rest[:at]
		rest = rest[at+len(word):]
		n := len([]rune(word))
		if n > available {
			if count > 0 {
				lines = append(lines, line)
			}
			lines, line, count = append(lines, word), indent, 0
			continue
		}
		if count > 0 && count+len([]rune(gap))+n > available {
			lines, line, count = append(lines, line), indent, 0
		}
		if count > 0 {
			line, count = line+gap, count+len([]rune(gap))
		}
		line, count = line+word, count+n
	}
	if count > 0 || len(lines) == 0 {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
func wrapMenuEntries(entries []string, width int) string {
	var lines []string
	line := ""
	for _, entry := range entries {
		if line != "" && len([]rune(line))+2+len([]rune(entry)) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		line += entry
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
func displayID(id string, width int) string {
	runes := []rune(id)
	available := width - 1
	if len(runes) <= available {
		return id
	}
	left := (available - 1) / 2
	return string(runes[:left]) + "…" + string(runes[len(runes)-(available-1-left):])
}
func terminalText(text string) string {
	width := terminalWidth()
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		clean := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, line)
		lines[i] = wrapWords(clean, width)
	}
	return strings.Join(lines, "\n")
}
func statePage(v Response, live string, details bool) string {
	var page strings.Builder
	r := v.Run
	s := r.Configs[len(r.Configs)-1]
	if s.Settings.Preset == "full" {
		fmt.Fprintln(&page, "Greenhouse | full pipeline")
	} else {
		fmt.Fprintln(&page, "Greenhouse | independent review")
	}
	fmt.Fprintf(&page, "Events: %s\n", live)
	fmt.Fprintf(&page, "Run %s\nStage: %s | %s\nCandidate: %s\nMode: %s | %s | limit %d\nReviewer: %s / %s\nFixer: %s / %s\nBudget: review %d + repair %d = %d of %d\nRules: %s\nSources: %s\n", r.ID, r.Stage, r.Status, r.Candidate, s.Settings.Mode, s.Settings.Preset, s.Settings.Limit, s.Settings.ReviewerBackend, s.Settings.ReviewerModel, s.Settings.FixerBackend, s.Settings.FixerModel, r.ReviewUsed, r.RepairUsed, r.ReviewUsed+r.RepairUsed, s.Settings.Limit, strings.Join(s.Settings.Rules, ", "), sourceSummary(fieldSources(s)))
	if r.Target != "" {
		fmt.Fprintf(&page, "Target: %s\nIntent: %s\nScope: committed target-to-candidate diff; dirty files excluded\n", r.Target, r.Intent)
	}
	if o := v.Repository; o != nil {
		fmt.Fprintln(&page, overrideLine(o))
	}
	for _, cfg := range v.Config {
		if cfg.Status == "retired" {
			fmt.Fprintf(&page, "Configuration: %s retired; preserved, not used for new reviews\n", cfg.File)
		}
	}
	if x := v.HostResult; x != nil {
		outcome := x.Verdict
		if outcome == "" {
			outcome = x.Status
		}
		fmt.Fprintln(&page, wrapWords(fmt.Sprintf("Host: %s | grant %s | %s | current %t", hostLabel(&x.Host), x.Grant, outcome, x.Current), terminalWidth()))
	}
	for _, rule := range v.Rules {
		fmt.Fprintf(&page, "Rule %s | %s | required=%t\nSource: %s\nDigest: %s\n", rule.Selection, rule.Status, rule.Required, rule.Source, rule.Digest)
		if rule.Error != "" {
			fmt.Fprintln(&page, "Load error: "+rule.Error)
		}
		if details && rule.Pack != nil {
			fmt.Fprintln(&page, v.RuleScope)
			for _, c := range rule.Pack.Criteria {
				fmt.Fprintf(&page, "%s/%s [%s]: %s\nRepair: %s\n", rule.Pack.ID, c.ID, c.Classification, c.Text, c.Repair)
			}
		}
	}
	fmt.Fprintf(&page, "Auto-resume: %s\n", autoResumeLabel(s.Settings))
	if p := r.Pause; p != nil {
		width := terminalWidth()
		fmt.Fprintln(&page, wrapWords("PAUSED: "+v.Explanation, width))
		fmt.Fprintln(&page, wrapWords(fmt.Sprintf("Paused stage: %s | agent %s %s | backend %s", p.Stage, p.Backend, p.Model, p.Reason), width))
		if p.ResetAt != "" {
			fmt.Fprintf(&page, "Reset: %s\n", clock(p.ResetAt))
		}
		if p.AutoResumeAt != "" {
			fmt.Fprintf(&page, "Scheduled %s: %s\n", p.AutoAction, clock(p.AutoResumeAt))
		}
	}
	for _, b := range v.Blocked {
		fmt.Fprintln(&page, "Blocked: "+b)
	}
	renderRecovery(&page, r)
	if r.Pipeline != nil {
		renderPipeline(&page, r, details)
	}
	if details && r.SelectedDiff != "" {
		fmt.Fprintln(&page, "Selected committed diff:", r.SelectedDiff)
	}
	actionable, informational := outstanding(r)
	label := map[string]string{}
	for _, f := range actionable {
		label[f.ID] = "Outstanding"
	}
	for _, f := range informational {
		label[f.ID] = "Informational"
	}
	kinds := resolvedKinds(r)
	responses := responseLines(r)
	for _, f := range r.Findings {
		state := label[f.ID]
		if state == "" {
			state = "Resolved (" + kinds[f.ID] + ")"
		}
		fmt.Fprintf(&page, "%s %s %s:%d %s | reviewed %s | ID %s\n", state, f.Priority, f.File, f.Start, f.Explanation, f.Head, displayID(f.ID, terminalWidth()))
		for _, line := range responses[f.ID] {
			fmt.Fprintln(&page, line)
		}
	}
	if r.Repair != nil {
		fix := r.Repair
		fmt.Fprintf(&page, "Repair: %s | handback: %s | actor: %s\nWriter: %s | base: %s | commit: %s\n", fix.Status, fix.Handback, fix.Actor, fix.Writer, fix.Base, fix.Head)
		fmt.Fprintf(&page, "Selected: %d | fixer input: %s | rules: %s | config: %s\n", len(fix.Selected), fix.InputDigest, fix.Rules.Digest, fix.ConfigDigest)
		if fix.Confinement != nil {
			fmt.Fprintln(&page, confinementLine(fix.Confinement))
		}
		for _, check := range fix.Checks {
			fmt.Fprintf(&page, "Check %s exit=%d: %s\n", strings.Join(check.Args, " "), check.Exit, check.Status)
		}
		if fix.Log != "" {
			fmt.Fprintln(&page, fix.Log)
		}
	}
	if r.Review != nil {
		x := r.Review
		fmt.Fprintf(&page, "Review: %s | %s | %s\nInput: %s\nCriteria: %s\nConfig: %s\n%s\n", x.Status, x.Backend, x.Config.Model, x.InputDigest, x.Rules.Digest, x.ConfigDigest, x.Log)
		if x.Snapshot != nil {
			fmt.Fprintln(&page, snapshotLine(x.Snapshot))
		}
		if x.Confinement != nil {
			fmt.Fprintln(&page, confinementLine(x.Confinement))
		}
		if details {
			fmt.Fprintf(&page, "Executor: %s\nArgs: %s\n", x.Config.Executable, strings.Join(x.Args, " "))
		}
		if x.Result != nil {
			fmt.Fprintf(&page, "Reported model: %s/%s\nCoverage: %s\n", x.Result.ReportedProvider, x.Result.ReportedModel, strings.Join(x.Result.Coverage, ", "))
			for _, f := range x.Result.Findings {
				fmt.Fprintf(&page, "%s %s %s:%d-%d %s\nAction: %s\nRule: %s | ID: %s\n", f.Priority, f.Origin, f.File, f.Start, f.End, f.Explanation, f.Action, f.Rule, displayID(f.ID, terminalWidth()))
			}
		}
	}
	return page.String()
}
func screen(text string) {
	fmt.Print("\033[2J\033[H")
	fmt.Println(terminalText(text))
}
func terminalPrompt(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	fmt.Print(terminalText(text))
	if strings.HasSuffix(text, " ") {
		fmt.Print(" ")
	}
}
func lineTerminal(h, path string) error {
	q := Request{Version: protocolMax, Op: "attach", Path: path}
	v, e := call(h, q)
	if e != nil {
		return e
	}
	if e = protocolRefusal(v); e != nil {
		return e
	}
	lines := make(chan string)
	go func() {
		reader := bufio.NewScanner(os.Stdin)
		for reader.Scan() {
			lines <- reader.Text()
		}
		close(lines)
	}()
	input := ""
	scan := func() bool {
		line, ok := <-lines
		input = line
		return ok
	}
	updates := make(chan liveUpdate, 1)
	stop := make(chan struct{})
	defer close(stop)
	go follow(h, path, v.Stream, v.Sequence, updates, stop)
	live := "live"
	message := ""
	details := false
	order := snapshotOrder{}
	for {
		if v.Run == nil {
			return errors.New(v.Error)
		}
		r := v.Run
		s := r.Configs[len(r.Configs)-1]
		var page strings.Builder
		page.WriteString(statePage(v, live, details))
		fmt.Fprintln(&page, "\n"+wrapMenuEntries([]string{"[i] Inspect", "[c] Criteria/diff", "[l] Load rules", "[s] Settings"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[p] Intent/target", "[v] Review", "[g] Pipeline", "[d] Decide"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[m] Resolve conflict", "[f] Selected repair", "[h] Handback"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[r] Rerun", "[a] Cancel auto-resume", "[b] Rerun from start"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[t] Trust repository", "[u] Migrate config", "[o] Logs"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[x] Cancel", "[n] New run", "[e] Recover", "[q] Detach"}, terminalWidth()))
		fmt.Fprint(&page, message)
		screen(page.String())
		terminalPrompt("> ")
		op, entered, redraw := "", false, false
		for !redraw && !entered {
			select {
			case u := <-updates:
				if u.view != nil && u.view.Run != nil && order.accepts(*u.view, v, false) {
					order.adopt(*u.view, v)
					v, redraw = *u.view, true
				}
				if u.state != live {
					live, redraw = u.state, true
				}
			case line, ok := <-lines:
				if !ok {
					return nil
				}
				op, entered = line, true
			}
		}
		if !entered {
			continue
		}
		if op == "q" {
			return nil
		}
		q = Request{Version: protocolMax, Op: "inspect", Path: path}
		if op == "o" {
			logged, err := call(h, Request{Version: protocolMax, Op: "transcript", Path: path, RunID: r.ID})
			if err != nil {
				return err
			}
			if logged.Transcript == nil {
				message = logged.Outcome + " " + logged.Error
				continue
			}
			screen(transcriptPage(r.ID, logged.Transcript))
			terminalPrompt("Press Enter to return\n> ")
			if !scan() {
				return nil
			}
			continue
		}
		if op == "c" {
			details = !details
		}
		if op == "v" {
			q.Op = "review"
		}
		if op == "x" {
			q.Op = "cancel"
		}
		if op == "f" {
			terminalPrompt("Finding IDs as shown (separated by commas or spaces):\n> ")
			if !scan() {
				return nil
			}
			var reviewed []string
			if r.Review != nil && r.Review.Result != nil {
				reviewed = findingIDs(r.Review.Result.Findings)
			}
			selected, whole, err := parseSelection(input, reviewed)
			if whole {
				err = errors.New("no finding ID entered")
			}
			if err != nil {
				message = err.Error() + "; nothing submitted"
				continue
			}
			q.Selected = selected
			q.Op = "repair"
		}
		if op == "h" {
			q.Op = "handback"
		}
		if op == "d" {
			if v.Decision == nil {
				message = "No open decision"
				continue
			}
			terminalPrompt("Choice (%s):\n> ", strings.Join(v.Decision.Options, ", "))
			if !scan() {
				return nil
			}
			q.Choice = input
			if !has(v.Decision.Options, q.Choice) {
				message = "unknown choice " + strconv.Quote(q.Choice) + "; enter an option exactly as listed; nothing submitted"
				continue
			}
			if q.Choice == "repair" && v.Decision.Kind == "findings" {
				terminalPrompt("Finding IDs as shown (separated by commas or spaces; an empty line selects the whole group):\n> ")
				if !scan() {
					return nil
				}
				selected, _, err := parseSelection(input, v.Decision.Findings)
				if err != nil {
					message = err.Error() + "; nothing submitted"
					continue
				}
				q.Selected = selected
			}
			if q.Choice == "increase-cap" {
				terminalPrompt("New limit:\n> ")
				if !scan() {
					return nil
				}
				if q.Limit, e = parseCount(input); e != nil {
					message = e.Error() + "; nothing submitted"
					continue
				}
			}
			terminalPrompt("Reason:\n> ")
			if !scan() {
				return nil
			}
			q.Reason = input
			q.Op = "decide"
		}
		if op == "e" {
			if r.Recovery == nil {
				message = "No open recovery"
				continue
			}
			terminalPrompt("Recovery (%s):\n> ", strings.Join(r.Recovery.Options, ", "))
			if !scan() {
				return nil
			}
			q.Choice = input
			if !has(r.Recovery.Options, q.Choice) {
				message = "unknown choice " + strconv.Quote(q.Choice) + "; enter an option exactly as listed; nothing submitted"
				continue
			}
			terminalPrompt("Reason:\n> ")
			if !scan() {
				return nil
			}
			q.Reason = input
			q.Op = "recover"
		}
		if op == "g" {
			q.Op = "pipeline"
		}
		if op == "m" {
			q.Op = "resolve-conflict"
		}
		if op == "p" {
			agents := installedAgents()
			if len(agents) == 0 {
				message = "No supported agent installed. Install pi, claude or codex on PATH and use its native login."
				continue
			}
			terminalPrompt("Installed agents: %s\nReviewer agent [%s] (Enter keeps current):\n> ", strings.Join(agents, ", "), s.Settings.ReviewerBackend)
			if !scan() {
				return nil
			}
			if input != "" && input != s.Settings.ReviewerBackend {
				if !has(agents, input) {
					message = "Choose an installed agent: " + strings.Join(agents, ", ")
					continue
				}
				draft := s.Settings
				draft.ReviewerBackend = input
				draft.FixerBackend = input
				draft.ReviewerModel = ""
				draft.FixerModel = ""
				applied, err := call(h, Request{Version: protocolMax, Op: "apply", Path: path, RunID: r.ID, Input: r.Input, Gate: currentGate(r), Expected: r.Token, Action: opaque(), Reason: "select installed agent", Settings: &draft})
				if err != nil {
					return err
				}
				accepted := order.accepts(applied, v, true)
				if accepted {
					v = applied
				}
				if applied.Outcome != "ok" {
					message = applied.Error
					continue
				}
				if !accepted {
					message = "Agent selection confirmed; waiting for follower state. Submit intent after resync."
					continue
				}
				r = v.Run
				q.RunID = r.ID
				q.Input = r.Input
				q.Expected = r.Token
				q.Gate = currentGate(r)
			}
			terminalPrompt("Intent:\n> ")
			if !scan() {
				return nil
			}
			q.Intent = input
			suggestion := suggestTarget(path)
			terminalPrompt("Review committed changes against Git target [%s]:\n> ", suggestion)
			if !scan() {
				return nil
			}
			q.Target = input
			if q.Target == "" {
				q.Target = suggestion
			}
			q.Op = "prepare"
		}
		if op == "l" {
			q.Op = "resolve-rules"
		}
		if op == "r" {
			q.Op = "rerun"
		}
		if op == "a" {
			q.Op = "cancel-auto-resume"
		}
		if op == "s" {
			models, modelErr := call(h, Request{Version: protocolMax, Op: "models", Path: path})
			modelsText := strings.Join(models.Models, ", ")
			if modelErr != nil {
				modelsText = "catalog unavailable: " + modelErr.Error()
			}
			terminalPrompt("Current values and sources:\n%s\nSettings draft (current %s %s %d)\nEnter mode preset limit, e.g. ask one-pass 6:\n> ", settingsTable(s), s.Settings.Mode, s.Settings.Preset, s.Settings.Limit)
			if !scan() {
				return nil
			}
			draft := s.Settings
			if draft.Mode, draft.Preset, draft.Limit, e = parseDraft(input); e != nil {
				message = "Invalid draft: " + e.Error()
				continue
			}
			terminalPrompt("Reviewer backend (pi, claude, codex) [%s]:\n> ", draft.ReviewerBackend)
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.ReviewerBackend = value
			}
			terminalPrompt("Pi models: %s\nEnter default to use the native model. Claude and Codex also accept an exact model ID.\nReviewer model [%s]:\n> ", modelsText, draft.ReviewerModel)
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.ReviewerModel = value
				if value == "default" {
					draft.ReviewerModel = ""
				}
			}
			terminalPrompt("Fixer backend (pi, claude, codex) [%s]:\n> ", draft.FixerBackend)
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.FixerBackend = value
			}
			terminalPrompt("Pi models: %s\nFixer model [%s]:\n> ", modelsText, draft.FixerModel)
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.FixerModel = value
				if value == "default" {
					draft.FixerModel = ""
				}
			}
			terminalPrompt("Selected rules (comma-separated; ? means optional; file:/absolute/path):\n> ")
			if !scan() {
				return nil
			}
			draft.Rules = []string{}
			for _, rule := range strings.Split(input, ",") {
				if name := strings.TrimSpace(rule); name != "" {
					draft.Rules = append(draft.Rules, name)
				}
			}
			terminalPrompt("Auto-resume (on, off; empty keeps %s):\n> ", autoResumeLabel(draft))
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.AutoResume = value
			}
			terminalPrompt("[a] Apply to run  [d] Save user defaults\n[r] Save repository override  [c] Cancel\n> ")
			if !scan() {
				return nil
			}
			switch input {
			case "a":
				q.Op = "apply"
			case "d":
				q.Op = "save"
			case "r":
				q.Op, q.Target = "save", "repository"
			default:
				continue
			}
			q.Settings = &draft
			q.Reason = "terminal Settings"
		} else if op == "t" {
			o := v.Repository
			if o == nil {
				continue
			}
			terminalPrompt("Trust %s of this repository (digest %.12s)? Its values apply to new runs. Type trust:\n> ", overrideName, o.Digest)
			if !scan() {
				return nil
			}
			if input != "trust" {
				continue
			}
			q.Op, q.Digest, q.Reason = "trust", o.Digest, "terminal trust"
		} else if op == "u" {
			terminalPrompt("Migrate older configuration files? Each keeps a backup. Type migrate:\n> ")
			if !scan() {
				return nil
			}
			if input != "migrate" {
				continue
			}
			q.Op, q.Reason = "migrate", "terminal migrate"
		} else if op == "b" {
			terminalPrompt("Keep this run and start a linked rerun from start? Type rerun:\n> ")
			if !scan() {
				return nil
			}
			if input != "rerun" {
				continue
			}
			q.Op = "rerun-from-start"
		} else if op == "n" {
			terminalPrompt("Retain evidence and start New run? Type new:\n> ")
			if !scan() {
				return nil
			}
			if input != "new" {
				continue
			}
			q.Op = "new"
		}
		if q.Op != "inspect" {
			q.Action = opaque()
			q.Client = "terminal"
			q.Expected = v.Token
			q.RunID = v.RunID
			q.Input = v.Input
			q.Gate = v.Gate
		}
		answer, e := call(h, q)
		if e != nil {
			return e
		}
		if order.accepts(answer, v, true) {
			v = answer
		}
		message = answer.Outcome + " " + answer.Error
	}
}
func Main() {
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" {
			fmt.Print(`Usage:
  greenhouse              Open the interactive terminal screen.
                          Use the line client when stdin or stdout is not a
                          terminal, or TERM is unset or dumb.
  greenhouse axi          Read one JSON request from stdin; write one JSON
                          response to stdout. Invoke once per request.
  greenhouse daemon       Run the local state daemon.
  greenhouse [-h|--help]   Show this help (also: greenhouse axi --help).

Host review (protocol version 2):
  attach (or new) with host identity, apply settings, resolve-rules,
  prepare with intent (what the change should do) and target (an exact commit).
  Review uses the merge-base of target and the candidate as the diff base.
  review requires a host grant for each review round; watch follows progress.
  Read host_result for the verdict and check current before accepting it.

Complete host requests and response fields: docs/embedding.md
`)
			return
		}
	}
	h, e := home()
	if e == nil {
		path, _ := os.Getwd()
		switch {
		case len(os.Args) == 1:
			e = terminal(h, path)
		case len(os.Args) == 2 && os.Args[1] == "daemon":
			e = daemon(h)
		case len(os.Args) == 2 && os.Args[1] == "axi":
			var q Request
			d := json.NewDecoder(io.LimitReader(os.Stdin, 65536))
			d.DisallowUnknownFields()
			e = d.Decode(&q)
			if e == nil {
				q.Path = path
				q.Client = "axi"
				var v Response
				v, e = call(h, q)
				if e == nil {
					e = json.NewEncoder(os.Stdout).Encode(v)
				}
			}
		default:
			e = errors.New("usage: greenhouse [axi|daemon]")
		}
	}
	if e != nil {
		if len(os.Args) > 1 && os.Args[1] == "axi" {
			json.NewEncoder(os.Stdout).Encode(Response{Version: 1, Outcome: "error", Error: e.Error()})
		} else {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}
}

func finishReview(r *Run, done reviewOutcome) {
	retainReviewMetadata(r.Review, done)
	status := done.Status
	r.Review.Thinking = done.Answer.Thinking
	recordAttempt(r, Attempt{Execution: r.Review.ID, Stage: "independent-review", Kind: "review", Role: "reviewer", Backend: r.Review.Config.Backend, Model: r.Review.Config.Model, Outcome: status, Reason: failureReason(done.Failure), Message: failureMessage(done.Failure), CostUSD: done.Answer.CostUSD, Tokens: done.Answer.Tokens, Thinking: done.Answer.Thinking})
	if done.Failure != nil && r.Status != "cancelling" {
		r.ReviewUsed--
		r.Review.Status, r.Review.Log = "paused", done.Log
		pauseRun(r, "independent-review", "review", "reviewer", r.Review.Config, r.Review.ID, done.Failure, time.Now())
		return
	}
	r.AutoRetries = 0
	if r.Status == "cancelling" {
		status = "cancelled"
	}
	id, err := resolve(r.Submitted.Worktree)
	if err != nil || id != r.Submitted {
		status = "stale"
	}
	r.Status, r.Review.Status, r.Review.Log = status, status, done.Log
	r.Review.Result = done.Result
	if done.Result != nil {
		index := map[string]int{}
		for i, f := range r.Findings {
			index[f.ID] = i
		}
		for _, f := range done.Result.Findings {
			if i, ok := index[f.ID]; ok {
				r.Findings[i] = f
			} else {
				index[f.ID] = len(r.Findings)
				r.Findings = append(r.Findings, f)
			}
		}
		if status == "clean-review" && len(r.Findings) > 0 {
			r.Status = "outstanding-findings"
		}
	}
	r.Token = opaque()
}

func repairHeld(x *Repair) bool {
	return x != nil && x.Handback != "handed-back" && x.Handback != "released"
}

func reviewArgsMatch(x *Execution) bool {
	expected := agentArgs(x.Config, "reviewer", verdictSchema(x.Full, len(x.Responses) > 0), reviewerPrompt)
	if !x.Config.Native {
		expected = legacyAgentArgs(x.Config, "reviewer", verdictSchema(x.Full, len(x.Responses) > 0), reviewerPrompt)
	}
	return digest(x.Args) == digest(expected)
}

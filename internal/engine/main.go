package engine

import (
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
	"strings"
	"syscall"
	"time"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

type executorDuration string

func (d *executorDuration) UnmarshalJSON(data []byte) error {
	var value *string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value == nil {
		return errors.New("executor_timeout must be a string")
	}
	*d = executorDuration(*value)
	return nil
}

type Settings struct {
	Mode            string           `json:"mode"`
	Preset          string           `json:"preset"`
	Limit           int              `json:"limit"`
	Rules           []string         `json:"rules"`
	ReviewerBackend string           `json:"reviewer_backend"`
	ReviewerModel   string           `json:"reviewer_model"`
	FixerBackend    string           `json:"fixer_backend"`
	FixerModel      string           `json:"fixer_model"`
	ExecutorTimeout executorDuration `json:"executor_timeout,omitempty"`
	AutoResume      string           `json:"auto_resume,omitempty"`
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
	StartedAt      string              `json:"started_at,omitempty"`
	StageStartedAt string              `json:"stage_started_at,omitempty"`
	Intent         string              `json:"intent,omitempty"`
	Reviewer       *ReviewerConfig     `json:"reviewer,omitempty"`
	SelectedDiff   string              `json:"selected_diff,omitempty"`
	Findings       []Finding           `json:"findings,omitempty"`
	Review         *Execution          `json:"review,omitempty"`
	RuleSnapshots  []RuleSnapshot      `json:"rule_snapshots,omitempty"`
	ID             string              `json:"id"`
	Input          string              `json:"input"`
	Token          string              `json:"token"`
	Submitted      repository.Identity `json:"submitted"`
	Target         string              `json:"target"`
	Candidate      string              `json:"candidate"`
	Stage          string              `json:"stage"`
	Status         string              `json:"status"`
	Configs        []Snapshot          `json:"configs"`
	ReviewUsed     int                 `json:"review_used"`
	RepairUsed     int                 `json:"repair_used"`
	Repair         *Repair             `json:"repair,omitempty"`
	Pipeline       *Pipeline           `json:"pipeline,omitempty"`
	Events         []Event             `json:"events"`
	Predecessor    string              `json:"predecessor,omitempty"`
	Successor      string              `json:"successor,omitempty"`
	Pause          *BackendPause       `json:"pause,omitempty"`
	Attempts       []Attempt           `json:"attempts,omitempty"`
	AutoRetries    int                 `json:"auto_retries,omitempty"`
	PriorRepairs   []Repair            `json:"prior_repairs,omitempty"`
	Recovery       *Recovery           `json:"recovery,omitempty"`
	Recoveries     []Recovery          `json:"recoveries,omitempty"`
	Inherited      *Inheritance        `json:"inherited,omitempty"`
	Host           *Host               `json:"host,omitempty"`
	HostFindings   []Finding           `json:"host_findings,omitempty"`
	HostCarried    []CarriedFinding    `json:"host_carried_findings,omitempty"`
	HostResponses  *Repair             `json:"host_responses,omitempty"`
	HostGrants     []string            `json:"host_grants,omitempty"`

	ReviewBase       string `json:"review_base,omitempty"`
	ReviewBaseReason string `json:"review_base_reason,omitempty"`
}
type Request struct {
	Step     string    `json:"step,omitempty"`
	Offset   *int64    `json:"offset,omitempty"`
	Full     bool      `json:"full,omitempty"`
	Follow   bool      `json:"follow,omitempty"`
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
	Title    string    `json:"title,omitempty"`
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

	Responses []ResolutionResponse `json:"responses,omitempty"`
}
type Response struct {
	ErrorCode   string            `json:"error_code,omitempty"`
	Retryable   *bool             `json:"retryable,omitempty"`
	Delivery    *DeliveryView     `json:"delivery,omitempty"`
	Logs        *LogsView         `json:"logs,omitempty"`
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
	home           string
	Version        int                `json:"version"`
	Defaults       Settings           `json:"defaults"`
	DefaultsSource string             `json:"defaults_source"`
	Runs           map[string]*Run    `json:"runs"`
	Slots          map[string]string  `json:"slots"`
	RunIndex       map[string]bool    `json:"run_index,omitempty"`
	ReceiptIndex   map[string]string  `json:"receipt_index,omitempty"`
	Receipts       map[string]Receipt `json:"receipts"`
	Migrations     []Migration        `json:"migrations,omitempty"`
	Trust          map[string]Trust   `json:"trust"`
}

func setRunStage(r *Run, stage string) {
	if r.Stage == stage && r.StageStartedAt != "" {
		return
	}
	r.Stage = stage
	r.StageStartedAt = time.Now().UTC().Format(time.RFC3339Nano)
}

func opaque() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func Digest(v any) string {
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
	return Digest(struct {
		ID       string   `json:"id"`
		Settings settings `json:"settings"`
		Source   string   `json:"source"`
	}{s.ID, settings{s.Settings.Mode, s.Settings.Preset, s.Settings.Limit, s.Settings.Rules}, s.Source})
}
func snapshotDigestMatches(s Snapshot, expected string) bool {
	return expected == Digest(s) || (s.LegacyDigest != "" && s.LegacyDigest == legacySnapshotDigest(s) && expected == s.LegacyDigest)
}
func reviewerConfigDigest(config ReviewerConfig) string {
	if config.Provider == (ProviderConfig{}) && config.Backend == "" {
		return Digest(struct {
			Executable string `json:"executable"`
			Model      string `json:"model"`
		}{config.Executable, config.Model})
	}
	return Digest(config)
}
func Home() (string, error) {
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
	_, err := persistSnapshot(h, s)
	return err
}

func persistSnapshot(h string, s *Store) ([]byte, error) {
	return persistSnapshotWithWriter(h, s, writeFileDurably)
}

func persistWithWriter(h string, s *Store, write func(string, []byte) error) error {
	_, err := persistSnapshotWithWriter(h, s, write)
	return err
}

func persistSnapshotWithWriter(h string, s *Store, write func(string, []byte) error) ([]byte, error) {
	garbage, err := compactHistory(h, s, write)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if err := write(filepath.Join(h, "state.json"), b); err != nil {
		return nil, err
	}
	for _, path := range garbage {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return b, nil
}

func load(h string) (*Store, error) {
	s := &Store{home: h, Version: 1, Defaults: Settings{Mode: "auto", Preset: "one-pass", Limit: 4, Rules: []string{"unslop", "no-comments"}, ReviewerBackend: defaultAgent(), FixerBackend: defaultAgent(), ExecutorTimeout: "45m"}, DefaultsSource: "built-in default", Runs: map[string]*Run{}, Slots: map[string]string{}, Receipts: map[string]Receipt{}, Trust: map[string]Trust{}}
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
			if r.ReviewUsed < 1 && x.Status != "paused" || x.ID == "" || x.Backend != executionBackends[BackendOf(x.Config)] || x.InputDigest != runner.BytesDigest(x.Input) || x.ConfigDigest != reviewerConfigDigest(x.Config) || x.Rules.Digest != runner.BytesDigest(x.Rules.Content) || (x.Rules.Candidate != r.Candidate && r.Pipeline == nil && (r.Repair == nil || x.Rules.Candidate != r.Repair.Base)) || !reviewArgsMatch(x) {
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
			r.Status = "needs-decision"
			setRunStage(r, "push")
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
				r.Status, r.Token = "running", opaque()
				setRunStage(r, "pipeline")
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
		id, e := repository.Resolve(r.Submitted.Worktree)
		v.Delivery = deliveryView(r, id, e)
		safe := e == nil && id.Repository == r.Submitted.Repository && id.Worktree == r.Submitted.Worktree && r.Status != "running" && r.Status != "cancelling" && r.Status != "writing" && r.Status != "recovery-required"
		held := custodyHeld(s, r)
		if held {
			v.Blocked = append(v.Blocked, "another run holds unpublished or ambiguous candidate custody for "+writerScope(r.Submitted)+"; writer actions blocked")
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
				if p := r.Pipeline; !held && p != nil && r.Configs[len(r.Configs)-1].Settings.Preset == "full" && p.Status != "ci-ready" && p.Status != "locally-ready" && !released(p) && p.Open() == nil {
					v.Actions = append(v.Actions, "pipeline")
				}
				if p := r.Pipeline; !held && p != nil && p.Open() != nil {
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
		if p := r.Pipeline; p != nil && p.Failed != "" && p.Status != "running" && p.Open() == nil {
			v.Blocked = append(v.Blocked, "pipeline stopped at "+p.Failed+": "+p.Reason)
		}
		if p := r.Pipeline; p != nil && p.Open() != nil {
			v.Decision = p.Open()
			v.Blocked = append(v.Blocked, "decision required: "+v.Decision.Kind+" ("+strings.Join(v.Decision.Options, ", ")+")")
		}
		if r.Host != nil {
			hostView(&v, r, e == nil && id == r.Submitted, safe && id == r.Submitted)
		}
	}
	return v
}
func BackendOf(config ReviewerConfig) string {
	if config.Backend == "" {
		return BackendPi
	}
	return config.Backend
}
func defaultAgent() string {
	agents := InstalledAgents()
	if len(agents) > 0 {
		return agents[0]
	}
	return BackendPi
}
func validModelSelection(backend, model string) bool {
	if model == "" {
		_, ok := executionBackends[backend]
		return ok
	}
	if backend == BackendClaude || backend == BackendCodex {
		if len(model) > 128 || model[0] == '-' || model[0] == '.' {
			return false
		}
		for _, c := range model {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '.' && c != '-' && c != '_' {
				return false
			}
		}
		return true
	}
	provider, modelID, qualified := strings.Cut(model, "/")
	return backend == "pi" && ValidText(model) && len(model) <= 256 && qualified && provider != "" && modelID != "" && !strings.ContainsAny(model, "*?[] \t\r\n:") && !strings.HasPrefix(model, "-")
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
func executorTimeout(s Settings) time.Duration {
	if s.ExecutorTimeout == "" {
		return runner.DefaultTimeout
	}
	d, _ := time.ParseDuration(string(s.ExecutorTimeout))
	return d
}

func valid(s Settings) bool {
	timeout, err := time.ParseDuration(string(s.ExecutorTimeout))
	if s.ExecutorTimeout != "" && (err != nil || timeout <= 0) {
		return false
	}
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
	if q.Op == "pipeline" && (q.WaitMS < 0 || q.WaitMS > 86400000) {
		return fail("pipeline wait_ms must be between 0 and 86400000", nil)
	}
	id, e := repository.Resolve(q.Path)
	if e != nil {
		return fail(e.Error(), nil)
	}
	if q.Responses != nil && (q.Host == nil || q.Op != "review") {
		return fail("responses require a host review request", nil)
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
	slot := Digest([]string{id.Repository, id.Worktree})
	r := s.Runs[s.Slots[slot]]
	if q.Op == "inspect" && q.RunID != "" {
		other, err := historicalRun(s, q.RunID)
		if err != nil {
			return fail("historical run unavailable: "+err.Error(), r)
		}
		if other == nil || other.Submitted.Repository != id.Repository {
			return fail("unknown run for this repository; historical data may have been pruned", r)
		}
		v := view(s, other)
		if other != r {
			v.Actions = []string{"attach", "inspect"}
		}
		return v
	}
	if q.Op == "logs" || q.Op == "transcript" {
		target := r
		if q.RunID != "" {
			var err error
			target, err = historicalRun(s, q.RunID)
			if err != nil {
				return fail("historical run unavailable: "+err.Error(), r)
			}
		}
		if target == nil || target.Submitted.Repository != id.Repository {
			return fail("unknown run for this repository; historical data may have been pruned", r)
		}
		if q.Op == "logs" {
			if q.WaitMS < 0 || q.WaitMS > maxWaitMillis {
				return fail("wait_ms must be between 0 and 60000", r)
			}
			logs, err := readLogs(target.ID, q)
			if err != nil {
				return fail(err.Error(), r)
			}
			v := view(s, target)
			v.Logs = logs
			return v
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
			r = &Run{StartedAt: time.Now().UTC().Format(time.RFC3339), ID: opaque(), Input: opaque(), Token: opaque(), Submitted: id, Candidate: id.Head, Status: "unconfigured", Configs: []Snapshot{initialSnapshot(s, id)}, Events: []Event{{Kind: "created", Actor: "client"}}, Host: q.Host}
			setRunStage(r, "intent")
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
	d := Digest(q)
	if receipt, ok, err := historicalReceipt(s, q.Action); err != nil {
		return fail("action receipt unavailable: "+err.Error(), r)
	} else if ok {
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
		if !ValidText(q.Reason) {
			return fail("reason required", r)
		}
		if q.Digest == status.PipelineDigest && status.PipelineStatus != "absent" {
			s.Trust[pipelineTrustKey(id)] = Trust{Repository: id.Repository, PipelinePath: status.PipelinePath, PipelineDigest: status.PipelineDigest, Actor: actor, Reason: q.Reason, Action: q.Action}
		} else if q.Digest == status.Digest {
			trust := s.Trust[id.Repository]
			trust.Repository, trust.Path, trust.Digest = id.Repository, status.Path, status.Digest
			trust.Actor, trust.Reason, trust.Action = actor, q.Reason, q.Action
			s.Trust[id.Repository] = trust
		} else {
			return fail("repository configuration changed or was not inspected; inspect it again before trusting", r)
		}
		v := view(s, r)
		s.Receipts[q.Action] = Receipt{d, frozen(v)}
		return v
	}
	if q.Op == "migrate" {
		if !ValidText(q.Reason) {
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
		if r.Status == "recovery-required" {
			return fail("run cannot progress; use recover and resolve its recovery options before retrying", r)
		}
		return fail("unsafe replacement or mutation; explicit recovery required", r)
	}
	paused := r
	crossHostNew := q.Op == "new" && q.Host != nil && r.Host != nil && !sameHost(r.Host, q.Host)
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
		if ownReviews(r) != 0 || r.Pipeline != nil && r.Pipeline.Worktree != "" {
			return fail("immutable review intent required before dispatch", r)
		}
		if len(q.Intent) > 8192 {
			v := fail("intent exceeds 8192-byte limit", r)
			v.ErrorCode = "intent_too_long"
			retryable := false
			v.Retryable = &retryable
			return v
		}
		if !ValidText(q.Intent) {
			return fail("immutable review intent required before dispatch", r)
		}
		if err := prepareRun(s, r, q.Intent, q.Target); err != nil {
			return fail(err.Error(), r)
		}
		if r.Host != nil && len(r.HostFindings) > 0 {
			if err := classifyHostFindings(r); err != nil {
				return fail(err.Error(), r)
			}
		}
		r.Token = opaque()
	} else if q.Op == "rerun-from-start" {
		if repairHeld(r.Repair) {
			return fail("repair custody remains unpublished or ambiguous; explicit handback required", r)
		}
		r = restartRun(s, r, id, actor)
		s.Slots[slot] = r.ID
	} else if q.Op == "review" && r.Host != nil {
		if err := admitHostReview(r, q.Grant, q.Responses); err != nil {
			return refusalResponse(view(s, r), err)
		}
	} else if q.Op == "review" {
		if err := admitReview(r); err != nil {
			return refusalResponse(view(s, r), err)
		}
	} else if q.Op == "repair" {
		if err := admitRepair(s, r, q); err != nil {
			return fail(err.Error(), r)
		}
	} else if q.Op == "pipeline" {
		if q.Intent != "" && !ValidText(q.Intent) {
			return fail("invalid intent", r)
		}
		if q.Title != "" && !validPullTitle(q.Title) {
			return fail("invalid PR title", r)
		}
		if q.Target != "" && r.Target != "" {
			target, err := repository.CommittedTarget(r.Submitted.Worktree, q.Target)
			if err != nil {
				return fail(err.Error(), r)
			}
			if target != r.Target {
				return fail("target is immutable after preparation", r)
			}
		}
		if q.Intent != "" && r.Pipeline == nil && ownReviews(r) == 0 {
			candidate := *r
			candidate.Configs = append([]Snapshot(nil), r.Configs...)
			settings := candidate.Configs[len(candidate.Configs)-1].Settings
			settings.Preset = "full"
			candidate.Configs = append(candidate.Configs, Snapshot{ID: opaque(), Settings: settings, Source: "pipeline intent"})
			if err := prepareRun(s, &candidate, q.Intent, q.Target); err != nil {
				return fail(err.Error(), r)
			}
			candidate.RuleSnapshots = append(candidate.RuleSnapshots, resolveRules(&candidate))
			*r = candidate
		} else if q.Intent != "" && q.Intent != r.Intent {
			return fail("intent is immutable after preparation", r)
		}
		if r.Pipeline == nil {
			return fail("prepare under the full preset first", r)
		}
		if r.Pipeline.Intent == "" {
			r.Pipeline.Intent = r.Intent
		}
		if q.Title != "" && r.Pipeline.PullTitle != "" && r.Pipeline.PullTitle != q.Title {
			return fail("PR title is immutable after pipeline start", r)
		}
		if err := admitPipeline(s, r); err != nil {
			return refusalResponse(view(s, r), err)
		}
		if r.Pipeline.PullTitle == "" {
			r.Pipeline.PullTitle = q.Title
			if r.Pipeline.PullTitle == "" {
				r.Pipeline.PullTitle = conventionalPullTitle(r.Pipeline.Intent)
			}
		}
		r.Token = opaque()
	} else if q.Op == "resolve-conflict" {
		if custodyHeld(s, r) {
			return fail(custodyError(r).Error(), r)
		}
		if err := admitConflictRepair(r, q); err != nil {
			return fail(err.Error(), r)
		}
		r.Events = append(r.Events, Event{Kind: "resolve-conflict", Actor: "client", Reason: "authorized fixer for target conflict"})
		r.Token = opaque()
	} else if q.Op == "decide" {
		var d *Decision
		if r.Pipeline != nil {
			d = r.Pipeline.Open()
		}
		if d == nil || d.ID != q.Gate {
			return fail("no open decision for this gate", r)
		}
		if custodyHeld(s, r) {
			return fail(custodyError(r).Error(), r)
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
			current := r.Configs[len(r.Configs)-1].Settings
			normalized := withModelDefaults(*q.Settings, current)
			if normalized.ExecutorTimeout == "" {
				normalized.ExecutorTimeout = current.ExecutorTimeout
				if normalized.ExecutorTimeout == "" {
					normalized.ExecutorTimeout = "45m"
				}
			}
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
				if err != nil && q.Settings.ReviewerBackend != BackendPi {
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
		var findings []Finding
		if !crossHostNew {
			findings = append(findings, r.Findings...)
		}
		next := &Run{StartedAt: time.Now().UTC().Format(time.RFC3339), Findings: findings, ID: opaque(), Input: opaque(), Token: opaque(), Submitted: id, Candidate: id.Head, Status: "unconfigured", Configs: []Snapshot{initialSnapshot(s, id)}, Events: []Event{{Kind: "new", Actor: "client", Reason: "replaces " + prior}}}
		setRunStage(next, "intent")
		if q.Host != nil {
			hostSuccessor(r, next, q.Host)
		}
		r = next
		s.Runs[r.ID] = r
		s.Slots[slot] = r.ID
	}
	if !crossHostNew {
		cancelAutoResume(paused, actor)
	}
	v := view(s, r)
	b, _ := json.Marshal(v)
	var frozen Response
	_ = json.Unmarshal(b, &frozen)
	s.Receipts[q.Action] = Receipt{d, frozen}
	return v
}
func frozen(v Response) Response {
	b, _ := json.Marshal(v)
	var out Response
	_ = json.Unmarshal(b, &out)
	return out
}
func acceptRepairProgress(r *Run, id string, value Repair) bool {
	if r == nil || r.Repair == nil || r.Repair.ID != id || r.Status == "cancelling" && value.Status == "committing" {
		return false
	}
	r.Repair = &value
	return true
}

func Daemon(h string) (retErr error) {
	f, e := os.OpenFile(filepath.Join(h, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return errors.Join(errors.New("daemon already owns state home"), f.Close())
	}
	defer func() {
		retErr = errors.Join(retErr, syscall.Flock(int(f.Fd()), syscall.LOCK_UN), f.Close())
	}()
	s, e := load(h)
	if e != nil {
		return e
	}
	committed, e := persistSnapshot(h, s)
	if e != nil {
		return e
	}
	socket := filepath.Join(h, "socket")
	if c, e := net.DialTimeout("unix", socket, time.Second); e == nil {
		return errors.Join(errors.New("live listener exists; refusing replacement"), c.Close())
	}
	if e = os.Remove(socket); e != nil && !os.IsNotExist(e) {
		return e
	}
	l, e := net.Listen("unix", socket)
	if e != nil {
		return e
	}
	defer func() {
		if err := l.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			retErr = errors.Join(retErr, err)
		}
	}()
	pid, _ := json.Marshal(os.Getpid())
	if e = writeFileDurably(filepath.Join(h, "pid"), pid); e != nil {
		return e
	}
	defer func() { _ = os.Remove(filepath.Join(h, "pid")) }() // Best-effort cleanup of daemon pid file.
	signalDone := make(chan struct{})
	defer close(signalDone)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			_ = l.Close() // Closing the listener unblocks Accept during shutdown.
		case <-signalDone:
		}
	}()
	type completion struct {
		finalization                bool
		run, execution, status, log string
		result                      *Verdict
		repair                      *Repair
		failure                     *BackendFailure
		answer                      agentAnswer
		snapshot                    string
		confinement                 *runner.Confinement
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
	pending := make(chan net.Conn, 64)
	defer func() {
		for len(pending) > 0 {
			_ = (<-pending).Close()
		}
	}()
	commit := func(next *Store) error {
		var nextCommitted []byte
		err := commitWhileServing(func() (*Store, error) {
			var snapshot Store
			if err := json.Unmarshal(committed, &snapshot); err != nil {
				return nil, err
			}
			snapshot.home = h
			return &snapshot, nil
		}, connections, pending, func() error {
			var err error
			nextCommitted, err = persistSnapshot(h, next)
			return err
		})
		if err != nil {
			return err
		}
		committed = nextCommitted
		return nil
	}
	stopped := make(chan struct{})
	defer close(stopped)
	active := map[string]context.CancelFunc{}
	pipelineDone := make(chan stepOutcome)
	pipelineProgress := make(chan repairProgress)
	reconcileHandbacks(s)
	reconcileRecoveries(s)
	releaseDeadLeases(s)
	if err := commit(s); err != nil {
		return err
	}
	defer func() {
		for _, cancel := range active {
			cancel()
		}
	}()
	processEvents := make(chan FeedEvent)
	emit := func(event FeedEvent) {
		select {
		case processEvents <- event:
		case <-stopped:
		}
	}
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
				if err := withWriterLock(r.Submitted, func() error { return ensureCandidate(r) }); err != nil {
					p.Active = nil
					if errors.Is(err, errCandidateUnknown) {
						holdRecovery(r, "candidate", step.Stage, step.ID, err.Error())
					} else {
						park(r, p.stage(step.Stage), "needs-decision", "private candidate unavailable: "+err.Error())
					}
					step = nil
				}
			}
			if err := commit(s); err != nil {
				return err
			}
			if step == nil {
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			active[r.ID] = cancel
			o := stepOutcome{Run: r.ID, Step: step.ID}
			ctx = runner.WithProcessTimeout(ctx, executorTimeout(r.Configs[len(r.Configs)-1].Settings))
			ctx = runner.WithExecutor(ctx, step.ID, Home)
			logStep := step.Stage
			if step.Kind == "repair" || step.Kind == "conflict" {
				logStep = "repair"
			}
			ctx = observeProcesses(ctx, r.ID, logStep, emit)
			pc, target, candidate, source := *p, r.Target, r.Candidate, r.Submitted
			var review Execution
			if r.Review != nil {
				review = *r.Review
				if step.Kind == "delivery" && step.Stage == "pr" && review.Result != nil {
					pc.addReview(&review)
				}
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
					started := time.Now()
					event := FeedEvent{Run: o.Run, Stage: step.Stage, Execution: step.ID, Command: []string{"greenhouse", step.Stage}, Kind: "step-started"}
					if step.Stage != "push" {
						emit(event)
					}
					o.Stage, o.Head, o.URL = runDeliveryStage(ctx, pc, step.Stage, candidate)
					o.Status, o.Log = o.Stage.Status, o.Stage.Reason
					if step.Stage != "push" {
						code := 0
						event.Kind = "step-finished"
						if o.Status != "passed" {
							code = 1
							event.Kind = "step-failed"
						}
						elapsed := time.Since(started).Milliseconds()
						event.ExitCode, event.ElapsedMS = &code, &elapsed
						emit(event)
					}
				case "review":
					reviewed := executeReview(ctx, &review)
					o.Finalization = reviewed.Finalization
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
				_ = c.Close() // Close the accepted connection during server shutdown.
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
				if err := commit(s); err != nil {
					return err
				}
				ctx, cancel := context.WithCancel(context.Background())
				active[r.ID] = cancel
				ctx = runner.WithProcessTimeout(ctx, executorTimeout(r.Configs[len(r.Configs)-1].Settings))
				ctx = runner.WithExecutor(ctx, x.ID, Home)
				ctx = observeProcesses(ctx, r.ID, "repair", emit)
				go func(runID string, repair Repair, source repository.Identity, ctx context.Context, cancel context.CancelFunc) {
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
			ctx = runner.WithProcessTimeout(ctx, executorTimeout(r.Configs[len(r.Configs)-1].Settings))
			ctx = observeProcesses(ctx, r.ID, "review", emit)
			runID, x := r.ID, *r.Review
			go func() {
				reviewed := executeReview(ctx, &x)
				cancel()
				select {
				case completed <- completion{finalization: reviewed.Finalization, run: runID, execution: x.ID, status: reviewed.Status, log: reviewed.Log, result: reviewed.Result, failure: reviewed.Failure, answer: reviewed.Answer, snapshot: reviewed.Snapshot, confinement: reviewed.Confinement}:
				case <-stopped:
				}
			}()
		}
		return schedule()
	}
	if err := launch(); err != nil {
		return err
	}
	wake := time.NewTimer(time.Hour)
	defer wake.Stop()
	orphanProbe := time.NewTicker(250 * time.Millisecond)
	defer orphanProbe.Stop()
	subscribers := time.NewTimer(time.Hour)
	defer subscribers.Stop()
	events.recordStage = logStageResult
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
		case event := <-processEvents:
			if len(event.Checks) > 0 {
				if r := s.Runs[event.Run]; r != nil && r.Pipeline != nil {
					if stage := r.Pipeline.stage("ci"); stage != nil {
						stage.Checks = event.Checks
					}
				}
			}
			events.append(event)
			continue
		case c = <-pending:
		case <-subscribers.C:
			continue
		case <-probe:
			if reconcileRecoveries(s) {
				if err := commit(s); err != nil {
					return err
				}
			}
			continue
		case <-wake.C:
			if fireAutoResumes(s, time.Now()) {
				if err := commit(s); err != nil {
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
				if err := commit(s); err != nil {
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
			if err := commit(s); err != nil {
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
				if err := commit(s); err != nil {
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
				if err := commit(s); err != nil {
					return err
				}
				continue
			}
			if r.Review == nil || r.Review.ID != done.execution {
				return errors.New("execution identity mismatch")
			}
			delete(active, r.ID)
			recordReviewEvidence(r.Review, done.snapshot, done.confinement)
			finishReview(r, reviewOutcome{Finalization: done.finalization, Status: done.status, Log: done.log, Result: done.result, Failure: done.failure, Answer: done.answer})
			if err := commit(s); err != nil {
				return err
			}
			continue
		case conn, ok := <-connections:
			if !ok {
				return nil
			}
			c = conn
		}
		if err := c.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			_ = c.Close() // A connection with no read timeout cannot be serviced safely.
			continue
		}
		var q Request
		dec := json.NewDecoder(io.LimitReader(c, 65536))
		dec.DisallowUnknownFields()
		e = dec.Decode(&q)
		var v Response
		committed := events.seq
		if e == nil && q.Op == "stop" {
			v = Response{Outcome: "ok"}
			if err := checkVersion(q); err != nil {
				v.Outcome, v.Error = "refused", err.Error()
			} else if len(active) > 0 || needsProbe(s) {
				v.Outcome, v.Error = "refused", "daemon is busy; let running work settle before stopping"
			}
			if v.Outcome == "ok" {
				v.Explanation = "daemon stopped"
			}
			stamp(&v, q.Version)
			if err := writeResponse(c, v); err != nil && v.Outcome != "ok" {
				continue
			}
			if v.Outcome == "ok" {
				return nil
			}
			continue
		}
		if e == nil && (q.Op == "watch" || q.Op == "logs") {
			var parked bool
			if v, parked = events.subscribe(s, c, q); parked {
				continue
			}
		} else if e != nil {
			v = Response{Version: 1, Outcome: "refused", Error: "invalid request: " + e.Error()}
		} else if q.Op == "inspect" || q.Op == "transcript" || q.Op == "models" {
			v = handle(s, q)
		} else {
			b, err := json.Marshal(s)
			if err != nil {
				return err
			}
			var next Store
			if err := json.Unmarshal(b, &next); err != nil {
				return err
			}
			next.home = h
			v = handle(&next, q)
			if e = commit(&next); e != nil {
				v = Response{Version: 1, Outcome: "error", Error: "storage commit failed: " + e.Error()}
				return errors.Join(e, writeResponse(c, v))
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
				_ = json.Unmarshal(b, &frozen)
				s.Receipts[q.Action] = Receipt{Digest: Digest(q), Result: frozen}
			}
			if q.Version >= 2 && !v.replayed {
				v.Stream, v.Sequence = events.stream, committed
				if receipt, ok, err := historicalReceipt(s, q.Action); err != nil {
					return err
				} else if ok && receipt.Digest == Digest(q) {
					receipt.Result.Stream, receipt.Result.Sequence = v.Stream, v.Sequence
					s.Receipts[q.Action] = receipt
				}
			}
			if e = commit(s); e != nil {
				return e
			}
			if e = launch(); e != nil {
				return e
			}
		}
		if q.Op == "pipeline" && q.WaitMS > 0 && v.Outcome == "ok" && executionActive(v.Run) {
			if len(events.waiters) >= waiterLimit {
				v.Wait = "elapsed"
			} else {
				q.RunID = v.RunID
				if err := c.SetDeadline(time.Time{}); err == nil {
					events.waiters = append(events.waiters, &waiter{conn: c, q: q, deadline: time.Now().Add(time.Duration(q.WaitMS) * time.Millisecond)})
					continue
				} else {
					v = Response{Outcome: "error", Error: err.Error()}
				}
			}
		}
		stamp(&v, q.Version)
		if err := writeResponse(c, v); err != nil {
			continue // A disconnected client must not stop the daemon.
		}
	}
}

func Send(h string, q Request, spawn bool) (v Response, retErr error) {
	if q.Op == "stop" {
		spawn = false
	}
	socket := filepath.Join(h, "socket")
	c, e := net.DialTimeout("unix", socket, time.Second)
	if e != nil && spawn {
		exe, x := os.Executable()
		if x != nil {
			return v, x
		}
		cmd := exec.Command(exe, "daemon")
		cmd.Dir = string(filepath.Separator)
		devNull, x := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if x != nil {
			return v, x
		}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
		cmd.Env = append(os.Environ(), "GREENHOUSE_HOME="+h)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if x = cmd.Start(); x != nil {
			_ = devNull.Close()
			return v, x
		}
		go func() { _ = cmd.Wait() }() // The daemon's lifecycle is observed through its socket.
		_ = devNull.Close()
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			c, e = net.DialTimeout("unix", socket, 100*time.Millisecond)
			if e == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if e != nil {
		if q.Op == "stop" && (errors.Is(e, syscall.ENOENT) || errors.Is(e, syscall.ECONNREFUSED)) {
			if err := waitDaemonStopped(h, false); err != nil {
				return v, err
			}
			v = Response{Outcome: "ok", Explanation: "daemon is not running"}
			if err := checkVersion(q); err != nil {
				v.Outcome, v.Error = "refused", err.Error()
			}
			stamp(&v, q.Version)
			return v, nil
		}
		return v, fmt.Errorf("daemon unavailable: %w", e)
	}
	defer func() { retErr = errors.Join(retErr, c.Close()) }()
	if err := c.SetDeadline(time.Now().Add(10*time.Second + time.Duration(q.WaitMS)*time.Millisecond)); err != nil {
		return v, err
	}
	if e = json.NewEncoder(c).Encode(q); e != nil {
		return v, e
	}
	e = json.NewDecoder(c).Decode(&v)
	if e == nil && q.Op == "stop" && checkVersion(q) == nil && v.Outcome == "refused" &&
		(v.Error == "unsupported operation: stop" || strings.HasPrefix(v.Error, "unsupported protocol version") && (v.Protocol == nil || v.Protocol.Max == 1)) {
		v.Error = "daemon does not support stop; let its work settle, then stop it with SIGTERM to the PID in GREENHOUSE_HOME/pid after confirming that process is the Greenhouse daemon"
		return v, nil
	}
	if e == nil && q.Op == "stop" && v.Outcome == "ok" {
		e = waitDaemonStopped(h, true)
	}
	return v, e
}

func writeResponse(conn net.Conn, response Response) error {
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return errors.Join(err, conn.Close())
	}
	return errors.Join(json.NewEncoder(conn).Encode(response), conn.Close())
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
	id, err := repository.Resolve(r.Submitted.Worktree)
	if err != nil || id != r.Submitted {
		status = "stale"
	}
	r.Status, r.Review.Status, r.Review.Log = status, status, done.Log
	r.Review.Result = done.Result
	if done.Result != nil {
		if r.Host != nil && (status == "clean-review" || status == "outstanding-findings") {
			r.HostFindings = append([]Finding{}, done.Result.Findings...)
		}
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
		dismissFindings(r, done.Result.Dismissed)
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
	expected, err := agentArgs(x.Config, "reviewer", verdictSchema(x.Full, len(x.Responses) > 0), reviewerPrompt)
	if err != nil {
		return false
	}
	if !x.Config.Native {
		expected = legacyAgentArgs(x.Config, "reviewer", verdictSchema(x.Full, len(x.Responses) > 0), reviewerPrompt)
	}
	if Digest(x.Args) == Digest(expected) {
		return true
	}
	if x.ReviewBudget != "" {
		return false
	}
	for i := range expected {
		expected[i] = strings.ReplaceAll(expected[i], reviewerPrompt, preBudgetReviewerPrompt)
	}
	return Digest(x.Args) == Digest(expected)
}

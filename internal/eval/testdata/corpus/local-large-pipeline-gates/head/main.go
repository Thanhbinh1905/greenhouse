package main

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
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"
)

type Settings struct {
	Mode   string   `json:"mode"`
	Preset string   `json:"preset"`
	Limit  int      `json:"limit"`
	Rules  []string `json:"rules"`
}
type Identity struct {
	Repository string `json:"repository"`
	Worktree   string `json:"worktree"`
	Branch     string `json:"branch"`
	Head       string `json:"head"`
}
type Snapshot struct {
	ID       string   `json:"id"`
	Settings Settings `json:"settings"`
	Source   string   `json:"source"`
}
type Event struct {
	Kind   string `json:"kind"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	Used   int    `json:"used"`
	Before int    `json:"before"`
	After  int    `json:"after"`
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
}
type Response struct {
	RuleScope  string         `json:"rule_scope,omitempty"`
	Rules      []RuleEvidence `json:"rules,omitempty"`
	RulesReady bool           `json:"rules_ready"`
	Version    int            `json:"version"`
	Outcome    string         `json:"outcome"`
	Error      string         `json:"error,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	Input      string         `json:"input_id,omitempty"`
	Gate       string         `json:"gate_id,omitempty"`
	Token      string         `json:"token,omitempty"`
	Run        *Run           `json:"run,omitempty"`
	Defaults   Settings       `json:"defaults"`
	Blocked    []string       `json:"blocked"`
	Actions    []string       `json:"actions"`
}
type Receipt struct {
	Digest string   `json:"digest"`
	Result Response `json:"result"`
}
type Store struct {
	Version  int                `json:"version"`
	Defaults Settings           `json:"defaults"`
	Runs     map[string]*Run    `json:"runs"`
	Slots    map[string]string  `json:"slots"`
	Receipts map[string]Receipt `json:"receipts"`
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
	s := &Store{Version: 1, Defaults: Settings{"auto", "one-pass", 4, []string{"unslop", "no-comments"}}, Runs: map[string]*Run{}, Slots: map[string]string{}, Receipts: map[string]Receipt{}}
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
	if s.Version != 1 || s.Runs == nil || s.Slots == nil || s.Receipts == nil {
		return nil, errors.New("unsupported or incomplete store")
	}
	for key, r := range s.Runs {
		if r == nil || key != r.ID || r.Input == "" || r.Token == "" || r.Candidate == "" || len(r.Configs) == 0 || r.ReviewUsed < 0 || r.RepairUsed < 0 {
			return nil, errors.New("invalid durable run")
		}
		if r.Review != nil {
			x := r.Review
			if r.ReviewUsed < 1 || x.ID == "" || x.Backend != "pi-json-v1" || x.InputDigest != bytesDigest(x.Input) || x.ConfigDigest != digest(x.Config) || x.Rules.Digest != bytesDigest(x.Rules.Content) || (x.Rules.Candidate != r.Candidate && r.Pipeline == nil && (r.Repair == nil || x.Rules.Candidate != r.Repair.Base)) || digest(x.Args) != digest(reviewerArgs(x.Config.Model)) {
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
		if r.Status == "running" || r.Status == "cancelling" || r.Status == "writing" {
			r.Status = "recovery-required"
			if p := r.Pipeline; p != nil {
				p.Status = "recovery-required"
				if p.Active != nil {
					st := p.stage(p.Active.Stage)
					st.Status, st.Reason = "unknown-outcome", "controller stopped during "+p.Active.Kind+"; no retry"
					p.Failed, p.Reason = st.Name, st.Reason
					if p.Active.Kind == "review" && r.Review != nil {
						r.Review.Status = "unknown-outcome"
					}
					if (p.Active.Kind == "repair" || p.Active.Kind == "conflict") && len(p.Repairs) > 0 {
						p.Repairs[len(p.Repairs)-1].Status = "unknown-outcome"
						p.Repairs[len(p.Repairs)-1].Writer = "unknown"
					}
				}
			} else if r.Review != nil {
				r.Review.Status = "unknown-outcome"
			}
			if r.Repair != nil && r.Repair.Status != "fixed-not-independently-rereviewed" {
				r.Repair.Status = "unknown-outcome"
				r.Repair.Writer = "unknown"
			}
			r.Token = opaque()
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
	v := Response{Version: 1, Outcome: "ok", Defaults: s.Defaults, Blocked: []string{"push, PR and CI unsupported; the full pipeline stops at local readiness", "repository overrides unsupported; repository configuration is untrusted"}, Actions: []string{"attach", "inspect"}}
	if r != nil {
		v.RuleScope = effectiveRuleScope(r)
		v.Rules, v.RulesReady = effectiveRules(r)
		if !v.RulesReady {
			v.Blocked = append(v.Blocked, "required rules unresolved or failed; use resolve-rules and inspect errors")
		}
		if r.Intent == "" || r.Target == "" || r.Reviewer == nil {
			v.Blocked = append(v.Blocked, "prepare requires explicit intent, exact target commit and private reviewer.json")
		}
		v.Run = r
		v.RunID = r.ID
		v.Input = r.Input
		v.Token = r.Token
		v.Gate = currentGate(r)
		id, e := resolve(r.Submitted.Worktree)
		safe := e == nil && id.Repository == r.Submitted.Repository && id.Worktree == r.Submitted.Worktree && r.Status != "running" && r.Status != "cancelling" && r.Status != "writing" && r.Status != "recovery-required"
		if safe {
			if id == r.Submitted {
				v.Actions = append(v.Actions, "apply", "save", "resolve-rules")
				if r.Status == "outstanding-findings" && r.Repair == nil && r.RepairUsed == 0 && r.ReviewUsed+r.RepairUsed < r.Configs[len(r.Configs)-1].Settings.Limit && repairable(r) {
					v.Actions = append(v.Actions, "repair")
				}
				if r.Repair != nil && r.Repair.Status == "fixed-not-independently-rereviewed" && (r.Repair.Handback == "awaiting-handback" || r.Repair.Handback == "handback-in-progress") {
					v.Actions = append(v.Actions, "handback")
				}
				if p := r.Pipeline; p != nil && r.Configs[len(r.Configs)-1].Settings.Preset == "full" && p.Status != "locally-ready" {
					v.Actions = append(v.Actions, "pipeline")
				}
				if p := r.Pipeline; p != nil && p.Gate != "" {
					v.Actions = append(v.Actions, "resolve-conflict")
				}
				if p := r.Pipeline; p != nil && p.Status == "locally-ready" && r.Candidate != r.Submitted.Head && p.Handback != "handed-back" {
					v.Actions = append(v.Actions, "handback")
				}
				if r.ReviewUsed == 0 && (r.Pipeline == nil || r.Pipeline.Worktree == "") {
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
		if r.Status == "running" || r.Status == "writing" && r.Repair != nil && (r.Repair.Status == "fixing" || r.Repair.Status == "checking") {
			v.Actions = append(v.Actions, "cancel")
		}
		if r.ReviewUsed+r.RepairUsed >= r.Configs[len(r.Configs)-1].Settings.Limit {
			v.Blocked = append(v.Blocked, "combined review and repair budget exhausted")
		}
		if r.Status == "outstanding-findings" && r.Repair == nil && !repairable(r) {
			v.Blocked = append(v.Blocked, "selected repair unavailable: actionable findings and private trusted fixer/check configuration are required")
		}
		if r.RepairUsed > 0 {
			v.Blocked = append(v.Blocked, "optional repair admission consumed; no second repair")
		}
		if r.Repair != nil && r.Repair.Status == "fixed-not-independently-rereviewed" {
			v.Blocked = append(v.Blocked, "fixed but not independently rereviewed")
		}
		if r.Review != nil && r.Review.Status == "clean-review" && (e != nil || id != r.Submitted) {
			v.Blocked = append(v.Blocked, "historical clean review only; source moved")
		}
		if r.Status == "recovery-required" {
			v.Blocked = append(v.Blocked, "unknown execution or writer outcome; recovery unsupported")
		}
		if p := r.Pipeline; p != nil && p.Failed != "" && p.Status != "running" {
			v.Blocked = append(v.Blocked, "pipeline stopped at "+p.Failed+": "+p.Reason)
		}
	}
	return v
}
func currentGate(r *Run) string {
	if r != nil && r.Pipeline != nil && r.Pipeline.Gate != "" {
		return r.Pipeline.Gate
	}
	return "none"
}
func pipelineCustody(r *Run) bool {
	p := r.Pipeline
	return p != nil && p.Worktree != "" && r.Candidate != r.Submitted.Head && p.Handback != "handed-back"
}
func valid(s Settings) bool {
	return (s.Mode == "auto" || s.Mode == "ask" || s.Mode == "yolo") && (s.Preset == "one-pass" || s.Preset == "full") && s.Limit >= 1 && s.Limit <= 1000 && len(s.Rules) <= 32
}
func handle(s *Store, q Request) Response {
	fail := func(msg string, r *Run) Response { v := view(s, r); v.Outcome = "refused"; v.Error = msg; return v }
	if q.Version != 1 {
		return fail("unsupported protocol version", nil)
	}
	id, e := resolve(q.Path)
	if e != nil {
		return fail(e.Error(), nil)
	}
	slot := digest([]string{id.Repository, id.Worktree})
	r := s.Runs[s.Slots[slot]]
	if q.Op == "attach" || q.Op == "inspect" {
		if r == nil && q.Op == "attach" {
			r = &Run{ID: opaque(), Input: opaque(), Token: opaque(), Submitted: id, Candidate: id.Head, Stage: "intent", Status: "unconfigured", Configs: []Snapshot{{opaque(), s.Defaults, "user defaults"}}, Events: []Event{{Kind: "created", Actor: "client"}}}
			s.Runs[r.ID] = r
			s.Slots[slot] = r.ID
		}
		return view(s, r)
	}
	if q.Op != "apply" && q.Op != "save" && q.Op != "new" && q.Op != "resolve-rules" && q.Op != "review" && q.Op != "prepare" && q.Op != "cancel" && q.Op != "repair" && q.Op != "handback" && q.Op != "pipeline" && q.Op != "resolve-conflict" {
		return fail("unsupported operation: "+q.Op, r)
	}
	if q.Action == "" {
		return fail("action_id required", r)
	}
	d := digest(q)
	if receipt, ok := s.Receipts[q.Action]; ok {
		if receipt.Digest != d {
			return fail("action_id reused with different input", r)
		}
		return receipt.Result
	}
	if r == nil || q.RunID != r.ID || q.Expected != r.Token || q.Input != r.Input || q.Gate != currentGate(r) {
		return fail("stale run/input/gate/token", r)
	}
	if q.Op == "new" && (r.Repair != nil && r.Repair.Handback != "handed-back" || pipelineCustody(r)) {
		return fail("repair custody remains unpublished or ambiguous; explicit handback required", r)
	}
	handbackHead := r.Candidate
	if r.Repair != nil {
		handbackHead = r.Repair.Head
	}
	handbackReconcile := q.Op == "handback" && (r.Repair != nil || r.Pipeline != nil) && id.Repository == r.Submitted.Repository && id.Worktree == r.Submitted.Worktree && id.Branch == r.Submitted.Branch && id.Head == handbackHead
	if q.Op != "new" && q.Op != "cancel" && id != r.Submitted && !handbackReconcile {
		return fail("source head or branch moved", r)
	}
	if q.Op == "cancel" && (r.Status == "running" || r.Status == "writing" && r.Repair != nil && (r.Repair.Status == "fixing" || r.Repair.Status == "checking")) {
		r.Status = "cancelling"
		r.Token = opaque()
	} else if r.Status == "running" || r.Status == "cancelling" || r.Status == "writing" || r.Status == "recovery-required" {
		return fail("unsafe replacement or mutation; explicit recovery required", r)
	}
	if q.Op == "cancel" {
		if r.Status != "cancelling" {
			return fail("no live execution", r)
		}
	} else if q.Op == "prepare" {
		if r.ReviewUsed != 0 || !validText(q.Intent) || r.Pipeline != nil && r.Pipeline.Worktree != "" {
			return fail("immutable review intent required before dispatch", r)
		}
		target, err := committedTarget(r.Submitted.Worktree, q.Target)
		if err != nil {
			return fail(err.Error(), r)
		}
		diff, err := gitRead(r.Submitted.Worktree, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", "--no-color", "--no-relative", "--src-prefix=a/", "--dst-prefix=b/", "--binary", target, r.Candidate, "--")
		if err != nil {
			return fail("cannot capture complete bounded diff", r)
		}
		cfg, err := trustedReviewer()
		if err != nil {
			return fail(err.Error(), r)
		}
		r.Pipeline = nil
		if r.Configs[len(r.Configs)-1].Settings.Preset == "full" {
			plan, digests, err := trustedPlan()
			if err != nil {
				return fail(err.Error(), r)
			}
			r.Pipeline = newPipeline(plan, digests)
		}
		r.Reviewer = &cfg
		r.SelectedDiff = string(diff)
		r.Intent, r.Target = q.Intent, target
		r.Token = opaque()
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
		if err := admitConflictRepair(r, q); err != nil {
			return fail(err.Error(), r)
		}
		r.Events = append(r.Events, Event{Kind: "resolve-conflict", Actor: "client", Reason: "authorized fixer for target conflict"})
		r.Token = opaque()
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
		if r.Review != nil {
			r.Status = "stale"
		}
		r.RuleSnapshots = append(r.RuleSnapshots, resolveRules(r))
		invalidate(r, "")
		r.Token = opaque()
	} else if q.Op == "apply" || q.Op == "save" {
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
			if r.Repair != nil {
				return fail("repair inputs and authority are immutable after authorization", r)
			}
			if r.Review != nil {
				r.Status = "stale"
			}
			old := r.Configs[len(r.Configs)-1].Settings
			r.Configs = append(r.Configs, Snapshot{opaque(), *q.Settings, "run-local"})
			invalidate(r, "")
			r.Events = append(r.Events, Event{q.Op, "client", q.Reason, r.ReviewUsed + r.RepairUsed, old.Limit, q.Settings.Limit})
		} else {
			s.Defaults = *q.Settings
			r.Events = append(r.Events, Event{Kind: "save", Actor: "client", Reason: q.Reason})
		}
		r.Token = opaque()
	} else {
		prior := r.ID
		findings := append([]Finding(nil), r.Findings...)
		r = &Run{Findings: findings, ID: opaque(), Input: opaque(), Token: opaque(), Submitted: id, Candidate: id.Head, Stage: "intent", Status: "unconfigured", Configs: []Snapshot{{opaque(), s.Defaults, "user defaults"}}, Events: []Event{{Kind: "new", Actor: "client", Reason: "replaces " + prior}}}
		s.Runs[r.ID] = r
		s.Slots[slot] = r.ID
	}
	v := view(s, r)
	b, _ := json.Marshal(v)
	var frozen Response
	json.Unmarshal(b, &frozen)
	s.Receipts[q.Action] = Receipt{d, frozen}
	return v
}
func acceptRepairProgress(r *Run, id string, value Repair) bool {
	if r == nil || r.Repair == nil || r.Repair.ID != id || r.Status == "cancelling" && value.Status == "committing" {
		return false
	}
	r.Repair = &value
	if r.Status == "cancelling" && value.Status == "cancelled" {
		r.Status = "cancelled"
		r.Token = opaque()
	}
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
	if err := persist(h, s); err != nil {
		return err
	}
	defer func() {
		for _, cancel := range active {
			cancel()
		}
	}()
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
					park(r, p.stage(step.Stage), "needs-decision", "private candidate unavailable: "+err.Error())
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
				case "review":
					o.Status, o.Result, o.Log = executeReview(ctx, &review)
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
	for {
		var c net.Conn
		select {
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
			status := done.status
			if r.Status == "cancelling" {
				status = "cancelled"
			}
			id, err := resolve(r.Submitted.Worktree)
			if err != nil || id != r.Submitted {
				status = "stale"
			}
			r.Status, r.Review.Status, r.Review.Log = status, status, done.log
			r.Review.Result = done.result
			if done.result != nil {
				seen := map[string]bool{}
				for _, f := range r.Findings {
					seen[f.ID] = true
				}
				for _, f := range done.result.Findings {
					if !seen[f.ID] {
						r.Findings = append(r.Findings, f)
						seen[f.ID] = true
					}
				}
				if status == "clean-review" && len(r.Findings) > 0 {
					r.Status = "outstanding-findings"
				}
			}
			r.Token = opaque()
			if err = persist(h, s); err != nil {
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
		if e != nil {
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
			if q.Op == "handback" && v.Outcome == "ok" {
				r := s.Runs[q.RunID]
				v = view(s, r)
				if r != nil && r.Repair != nil && r.Repair.Handback == "awaiting-handback" {
					v.Outcome = "refused"
					v.Error = r.Repair.Log
				}
				if r != nil && r.Repair == nil && r.Pipeline != nil && r.Pipeline.Handback == "awaiting-handback" {
					v.Outcome = "refused"
					v.Error = r.Pipeline.HandbackError
				}
				b, _ := json.Marshal(v)
				var frozen Response
				json.Unmarshal(b, &frozen)
				s.Receipts[q.Action] = Receipt{Digest: digest(q), Result: frozen}
			}
			if e = persist(h, s); e != nil {
				return e
			}
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
					status, result, log := executeReview(ctx, &x)
					cancel()
					select {
					case completed <- completion{run: runID, execution: x.ID, status: status, log: log, result: result}:
					case <-stopped:
					}
				}()
			}
			if e = schedule(); e != nil {
				return e
			}
		}
		json.NewEncoder(c).Encode(v)
		c.Close()
	}
}
func call(h string, q Request) (Response, error) {
	var v Response
	socket := filepath.Join(h, "socket")
	c, e := net.DialTimeout("unix", socket, time.Second)
	if e != nil {
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
	c.SetDeadline(time.Now().Add(10 * time.Second))
	if e = json.NewEncoder(c).Encode(q); e != nil {
		return v, e
	}
	e = json.NewDecoder(c).Decode(&v)
	return v, e
}
func screen(text string) {
	size := [4]uint16{}
	width := 80
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size))); err == 0 && size[1] > 1 {
		width = int(size[1]) - 1
	}
	fmt.Print("\033[2J\033[H")
	for _, line := range strings.Split(text, "\n") {
		clean := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, line)
		count := 0
		for _, r := range clean {
			if count == width {
				fmt.Println()
				count = 0
			}
			fmt.Print(string(r))
			count++
		}
		fmt.Println()
	}
}
func terminal(h, path string) error {
	q := Request{Version: 1, Op: "attach", Path: path}
	v, e := call(h, q)
	if e != nil {
		return e
	}
	reader := bufio.NewScanner(os.Stdin)
	message := ""
	details := false
	for {
		var page strings.Builder
		if v.Run == nil {
			return errors.New(v.Error)
		}
		r := v.Run
		s := r.Configs[len(r.Configs)-1]
		if s.Settings.Preset == "full" {
			fmt.Fprintln(&page, "Greenhouse | full pipeline")
		} else {
			fmt.Fprintln(&page, "Greenhouse | independent review")
		}
		fmt.Fprintf(&page, "Run %s\nStage: %s | %s\nCandidate: %s\nMode: %s | %s | limit %d\nBudget: review %d + repair %d\nRules: %s\nSettings source: %s\n", r.ID, r.Stage, r.Status, r.Candidate, s.Settings.Mode, s.Settings.Preset, s.Settings.Limit, r.ReviewUsed, r.RepairUsed, strings.Join(s.Settings.Rules, ", "), s.Source)
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
		for _, b := range v.Blocked {
			fmt.Fprintln(&page, "Blocked: "+b)
		}
		if r.Pipeline != nil {
			renderPipeline(&page, r, details)
		}
		if details && r.SelectedDiff != "" {
			fmt.Fprintln(&page, "Selected committed diff:", r.SelectedDiff)
		}
		for _, f := range r.Findings {
			fmt.Fprintf(&page, "Outstanding %s %s:%d %s | reviewed %s\n", f.Priority, f.File, f.Start, f.Explanation, f.Head)
		}
		if r.Repair != nil {
			fix := r.Repair
			fmt.Fprintf(&page, "Repair: %s | handback: %s | actor: %s\nWriter: %s | base: %s | commit: %s\n", fix.Status, fix.Handback, fix.Actor, fix.Writer, fix.Base, fix.Head)
			fmt.Fprintf(&page, "Selected: %d | fixer input: %s | rules: %s | config: %s\n", len(fix.Selected), fix.InputDigest, fix.Rules.Digest, fix.ConfigDigest)
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
			if details {
				fmt.Fprintf(&page, "Executor: %s\nArgs: %s\n", x.Config.Executable, strings.Join(x.Args, " "))
			}
			if x.Result != nil {
				fmt.Fprintf(&page, "Reported model: %s/%s\nCoverage: %s\n", x.Result.ReportedProvider, x.Result.ReportedModel, strings.Join(x.Result.Coverage, ", "))
				for _, f := range x.Result.Findings {
					fmt.Fprintf(&page, "%s %s %s:%d-%d %s\nAction: %s\nRule: %s | ID: %s\n", f.Priority, f.Origin, f.File, f.Start, f.End, f.Explanation, f.Action, f.Rule, f.ID)
				}
			}
		}
		fmt.Fprintln(&page, "\n[i] Inspect  [c] Criteria/diff  [r] Resolve rules  [s] Settings\n[p] Intent/target  [v] Review  [g] Pipeline  [m] Resolve conflict\n[f] Selected repair  [h] Handback  [x] Cancel  [n] New run  [q] Detach")
		fmt.Fprint(&page, message)
		screen(page.String())
		fmt.Print("> ")
		if !reader.Scan() {
			return reader.Err()
		}
		op := strings.TrimSpace(reader.Text())
		if op == "q" {
			return nil
		}
		q = Request{Version: 1, Op: "inspect", Path: path}
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
			fmt.Print("Selected finding IDs (comma-separated):\n> ")
			if !reader.Scan() {
				return nil
			}
			for _, id := range strings.Split(reader.Text(), ",") {
				if id = strings.TrimSpace(id); id != "" {
					q.Selected = append(q.Selected, id)
				}
			}
			q.Op = "repair"
		}
		if op == "h" {
			q.Op = "handback"
		}
		if op == "g" {
			q.Op = "pipeline"
		}
		if op == "m" {
			q.Op = "resolve-conflict"
		}
		if op == "p" {
			fmt.Print("Intent:\n> ")
			if !reader.Scan() {
				return nil
			}
			q.Intent = reader.Text()
			fmt.Print("Exact target commit ID:\n> ")
			if !reader.Scan() {
				return nil
			}
			q.Target = reader.Text()
			q.Op = "prepare"
		}
		if op == "r" {
			q.Op = "resolve-rules"
		}
		if op == "s" {
			fmt.Printf("Settings draft (current %s %s %d)\nEnter mode preset limit, e.g. ask one-pass 6:\n> ", s.Settings.Mode, s.Settings.Preset, s.Settings.Limit)
			if !reader.Scan() {
				return nil
			}
			draft := s.Settings
			if _, e = fmt.Sscan(reader.Text(), &draft.Mode, &draft.Preset, &draft.Limit); e != nil {
				message = "Invalid draft"
				continue
			}
			fmt.Print("Selected rules (comma-separated; ? means optional; file:/absolute/path):\n> ")
			if !reader.Scan() {
				return nil
			}
			draft.Rules = []string{}
			for _, rule := range strings.Split(reader.Text(), ",") {
				if name := strings.TrimSpace(rule); name != "" {
					draft.Rules = append(draft.Rules, name)
				}
			}
			fmt.Print("[a] Apply to run  [d] Save user defaults  [c] Cancel\n> ")
			if !reader.Scan() {
				return nil
			}
			switch strings.TrimSpace(reader.Text()) {
			case "a":
				q.Op = "apply"
			case "d":
				q.Op = "save"
			default:
				continue
			}
			q.Settings = &draft
			q.Reason = "terminal Settings"
		} else if op == "n" {
			fmt.Print("Retain evidence and start New run? Type new:\n> ")
			if !reader.Scan() {
				return nil
			}
			if reader.Text() != "new" {
				continue
			}
			q.Op = "new"
		}
		if q.Op != "inspect" {
			q.Action = opaque()
			q.Expected = v.Token
			q.RunID = v.RunID
			q.Input = v.Input
			q.Gate = v.Gate
		}
		v, e = call(h, q)
		if e != nil {
			return e
		}
		message = v.Outcome + " " + v.Error
	}
}
func main() {
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

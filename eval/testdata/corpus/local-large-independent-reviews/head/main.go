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
			if r.ReviewUsed < 1 || x.ID == "" || x.Backend != "pi-json-v1" || x.InputDigest != bytesDigest(x.Input) || x.ConfigDigest != digest(x.Config) || x.Rules.Digest != bytesDigest(x.Rules.Content) || x.Rules.Candidate != r.Candidate || digest(x.Args) != digest(reviewerArgs(x.Config.Model)) {
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
			if r.Review != nil {
				r.Review.Status = "unknown-outcome"
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
	v := Response{Version: 1, Outcome: "ok", Defaults: s.Defaults, Blocked: []string{"repair, full pipeline, tests, docs, lint, push, PR and CI unsupported", "repository overrides unsupported; repository configuration is untrusted"}, Actions: []string{"attach", "inspect"}}
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
		v.Gate = "none"
		id, e := resolve(r.Submitted.Worktree)
		safe := e == nil && id.Repository == r.Submitted.Repository && id.Worktree == r.Submitted.Worktree && r.Status != "running" && r.Status != "cancelling" && r.Status != "writing" && r.Status != "recovery-required"
		if safe {
			if id == r.Submitted {
				v.Actions = append(v.Actions, "apply", "save", "resolve-rules")
				if r.ReviewUsed == 0 {
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
		if r.Status == "running" {
			v.Actions = append(v.Actions, "cancel")
		}
		if r.ReviewUsed >= r.Configs[len(r.Configs)-1].Settings.Limit {
			v.Blocked = append(v.Blocked, "review budget exhausted")
		}
		if r.ReviewUsed > 0 {
			v.Blocked = append(v.Blocked, "one-pass invocation consumed; no automatic retry or repair")
		}
		if r.Review != nil && r.Review.Status == "clean-review" && (e != nil || id != r.Submitted) {
			v.Blocked = append(v.Blocked, "historical clean review only; source moved")
		}
		if r.Status == "recovery-required" {
			v.Blocked = append(v.Blocked, "unknown execution or writer outcome; recovery unsupported")
		}
	}
	return v
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
	if q.Op != "apply" && q.Op != "save" && q.Op != "new" && q.Op != "resolve-rules" && q.Op != "review" && q.Op != "prepare" && q.Op != "cancel" {
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
	if r == nil || q.RunID != r.ID || q.Expected != r.Token || q.Input != r.Input || q.Gate != "none" {
		return fail("stale run/input/gate/token", r)
	}
	if q.Op != "new" && q.Op != "cancel" && id != r.Submitted {
		return fail("source head or branch moved", r)
	}
	if q.Op == "cancel" && r.Status == "running" {
		r.Status = "cancelling"
		r.Token = opaque()
	} else if r.Status == "running" || r.Status == "cancelling" || r.Status == "writing" || r.Status == "recovery-required" {
		return fail("unsafe replacement or mutation; explicit recovery required", r)
	}
	if q.Op == "cancel" {
		if r.Status != "cancelling" {
			return fail("no live review", r)
		}
	} else if q.Op == "prepare" {
		if r.ReviewUsed != 0 || !validText(q.Intent) {
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
		r.Reviewer = &cfg
		r.SelectedDiff = string(diff)
		r.Intent, r.Target = q.Intent, target
		r.Token = opaque()
	} else if q.Op == "review" {
		if err := admitReview(r); err != nil {
			return fail(err.Error(), r)
		}
	} else if q.Op == "resolve-rules" {
		if r.Review != nil {
			r.Status = "stale"
		}
		r.RuleSnapshots = append(r.RuleSnapshots, resolveRules(r))
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
			if r.Review != nil {
				r.Status = "stale"
			}
			old := r.Configs[len(r.Configs)-1].Settings
			r.Configs = append(r.Configs, Snapshot{opaque(), *q.Settings, "run-local"})
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
	}
	completed := make(chan completion)
	connections := make(chan net.Conn)
	stopped := make(chan struct{})
	defer close(stopped)
	active := map[string]context.CancelFunc{}
	defer func() {
		for _, cancel := range active {
			cancel()
		}
	}()
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
		case done := <-completed:
			r := s.Runs[done.run]
			if r == nil || r.Review == nil || r.Review.ID != done.execution {
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
			for _, r := range s.Runs {
				if r.Status == "cancelling" {
					if cancel := active[r.ID]; cancel != nil {
						cancel()
					}
					continue
				}
				if r.Status != "running" || r.Review == nil || active[r.ID] != nil {
					continue
				}
				ctx, cancel := context.WithCancel(context.Background())
				active[r.ID] = cancel
				runID, x := r.ID, *r.Review
				go func() {
					status, result, log := executeReview(ctx, &x)
					cancel()
					select {
					case completed <- completion{runID, x.ID, status, log, result}:
					case <-stopped:
					}
				}()
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
		fmt.Fprintln(&page, "Greenhouse | independent review")
		if v.Run == nil {
			return errors.New(v.Error)
		}
		r := v.Run
		s := r.Configs[len(r.Configs)-1]
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
		if details && r.SelectedDiff != "" {
			fmt.Fprintln(&page, "Selected committed diff:", r.SelectedDiff)
		}
		for _, f := range r.Findings {
			fmt.Fprintf(&page, "Outstanding %s %s:%d %s | reviewed %s\n", f.Priority, f.File, f.Start, f.Explanation, f.Head)
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
		fmt.Fprintln(&page, "\n[i] Inspect  [c] Criteria/diff  [r] Resolve rules  [s] Settings\n[p] Intent/target  [v] Review  [x] Cancel  [n] New run  [q] Detach")
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

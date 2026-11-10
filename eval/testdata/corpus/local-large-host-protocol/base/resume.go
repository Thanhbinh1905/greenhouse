package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type BackendPause struct {
	Stage        string         `json:"stage"`
	Kind         string         `json:"kind"`
	Role         string         `json:"role"`
	Backend      string         `json:"backend"`
	Model        string         `json:"model"`
	Reason       string         `json:"reason"`
	Message      string         `json:"message"`
	ResetAt      string         `json:"reset_at,omitempty"`
	At           string         `json:"at"`
	Execution    string         `json:"execution"`
	AutoResumeAt string         `json:"auto_resume_at,omitempty"`
	AutoAction   string         `json:"auto_action,omitempty"`
	AutoTo       *BackendChoice `json:"auto_to,omitempty"`
	AutoNote     string         `json:"auto_note,omitempty"`
}
type Attempt struct {
	Execution string  `json:"execution"`
	Stage     string  `json:"stage"`
	Kind      string  `json:"kind"`
	Role      string  `json:"role"`
	Backend   string  `json:"backend"`
	Model     string  `json:"model"`
	Outcome   string  `json:"outcome"`
	Reason    string  `json:"reason,omitempty"`
	Message   string  `json:"message,omitempty"`
	CostUSD   float64 `json:"cost_usd"`
	Tokens    int     `json:"tokens"`
	Partial   string  `json:"partial,omitempty"`
	At        string  `json:"at"`
}

var backendNames = map[string]string{backendPi: "Pi", backendClaude: "Claude", backendCodex: "Codex"}

func recordAttempt(r *Run, a Attempt) {
	if a.Backend == "" {
		a.Backend = backendPi
	}
	a.At = time.Now().UTC().Format(time.RFC3339)
	r.Attempts = append(r.Attempts, a)
}

func pauseRun(r *Run, stage, kind, role string, config ReviewerConfig, execution string, f *BackendFailure, now time.Time) {
	r.Pause = &BackendPause{Stage: stage, Kind: kind, Role: role, Backend: backendOf(config), Model: config.Model, Reason: f.Reason, Message: f.Message, ResetAt: f.ResetAt, At: now.UTC().Format(time.RFC3339), Execution: execution}
	r.Status = "paused"
	r.Stage = stage
	r.Token = opaque()
	r.Events = append(r.Events, Event{Kind: "paused", Actor: "daemon", Reason: "backend " + f.Reason + ": " + f.Message})
	planAutoResume(r, now)
}

func autoResumeOn(r *Run) bool {
	settings := r.Configs[len(r.Configs)-1].Settings
	return settings.Mode != "ask" && settings.AutoResume != "off"
}

func autoActor(r *Run) string {
	if r.Configs[len(r.Configs)-1].Settings.Mode == "yolo" && r.Pipeline != nil {
		if actor, ok := yoloActor(r); ok {
			return actor
		}
	}
	return "auto"
}

func nextFallback(r *Run, role string) *BackendChoice {
	config, err := trustedBackends()
	if err != nil {
		return nil
	}
	settings := r.Configs[len(r.Configs)-1].Settings
	current := BackendChoice{settings.ReviewerBackend, settings.ReviewerModel}
	if role == "fixer" {
		current = BackendChoice{settings.FixerBackend, settings.FixerModel}
	}
	choices := config.Fallback[role]
	start := 0
	for i, choice := range choices {
		if choice == current {
			start = i + 1
		}
	}
	for _, choice := range choices[start:] {
		if choice != current {
			c := choice
			return &c
		}
	}
	return nil
}

func planAutoResume(r *Run, now time.Time) {
	p := r.Pause
	p.AutoResumeAt, p.AutoAction, p.AutoTo, p.AutoNote = "", "", nil, ""
	switch {
	case r.Configs[len(r.Configs)-1].Settings.Mode == "ask":
		p.AutoNote = "Ask mode never resumes automatically"
		if fallback := nextFallback(r, p.Role); fallback != nil && p.Reason == "quota" {
			p.AutoNote += fmt.Sprintf("; fallback %s %s is configured", fallback.Backend, fallback.Model)
		}
		return
	case !autoResumeOn(r):
		p.AutoNote = "Auto-resume is off"
		return
	case p.Reason == "auth":
		p.AutoNote = "Automatic resume cannot fix a missing login"
		return
	}
	if fallback := nextFallback(r, p.Role); fallback != nil && p.Reason == "quota" {
		p.AutoResumeAt, p.AutoAction, p.AutoTo = now.UTC().Format(time.RFC3339), "fallback", fallback
		return
	}
	if p.ResetAt != "" {
		if at, err := time.Parse(time.RFC3339, p.ResetAt); err == nil {
			p.AutoResumeAt, p.AutoAction = at.Add(5*time.Second).UTC().Format(time.RFC3339), "resume"
			return
		}
	}
	policy := retryPolicy()
	if r.AutoRetries >= policy.MaxAttempts {
		p.AutoNote = fmt.Sprintf("Automatic retries are exhausted (%d of %d)", r.AutoRetries, policy.MaxAttempts)
		return
	}
	delay := time.Duration(policy.BaseSeconds) * time.Second << r.AutoRetries
	if limit := time.Duration(policy.MaxSeconds) * time.Second; delay > limit || delay <= 0 {
		delay = limit
	}
	p.AutoResumeAt, p.AutoAction = now.Add(delay).UTC().Format(time.RFC3339), "resume"
}

func cancelAutoResume(r *Run, actor string) {
	if r.Pause == nil || r.Pause.AutoResumeAt == "" {
		return
	}
	r.Pause.AutoResumeAt, r.Pause.AutoAction, r.Pause.AutoTo = "", "", nil
	r.Pause.AutoNote = "Automatic resume cancelled by " + actor
	r.Events = append(r.Events, Event{Kind: "auto-resume-cancelled", Actor: actor, Reason: "manual action or settings change"})
}

func clock(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return at
	}
	t = t.Local()
	if now := time.Now(); t.YearDay() != now.YearDay() || t.Year() != now.Year() {
		return t.Format("Jan 2 15:04")
	}
	return t.Format("15:04")
}

func pauseSentence(r *Run) string {
	p := r.Pause
	if p == nil {
		return ""
	}
	name := backendNames[p.Backend]
	what := map[string]string{"quota": name + " quota exhausted.", "auth": name + " login missing; run the " + p.Backend + " CLI to log in.", "unavailable": name + " is unavailable."}[p.Reason]
	if p.Reason == "quota" && p.ResetAt != "" {
		what = strings.TrimSuffix(what, ".") + "; it resets at " + clock(p.ResetAt) + "."
	}
	mode := r.Configs[len(r.Configs)-1].Settings.Mode
	next := ""
	switch {
	case p.AutoResumeAt != "" && p.AutoAction == "fallback":
		next = fmt.Sprintf(" %s mode will switch to %s %s now.", strings.ToUpper(mode[:1])+mode[1:], p.AutoTo.Backend, p.AutoTo.Model)
	case p.AutoResumeAt != "":
		next = fmt.Sprintf(" %s mode will resume at %s with the same agent.", strings.ToUpper(mode[:1])+mode[1:], clock(p.AutoResumeAt))
	case p.AutoNote != "":
		next = " " + p.AutoNote + "."
	}
	keys := " Press r to rerun now, s to change agent."
	if p.AutoResumeAt != "" {
		keys = " Press r to rerun now, a to cancel auto-resume, s to change agent."
	}
	return what + next + keys
}

func resumeRun(s *Store, r *Run, actor string) error {
	p := r.Pause
	if p == nil {
		return errors.New("no backend pause to resume")
	}
	r.Pause = nil
	var err error
	switch {
	case r.Pipeline != nil:
		err = resumePipeline(s, r, p)
	case p.Kind == "review":
		err = admitReview(r)
	case r.Repair != nil && r.Repair.ID == p.Execution:
		var x *Repair
		if x, err = readmitRepair(r, *r.Repair); err == nil {
			r.Repair = x
			r.Status, r.Stage = "writing", "selected-repair"
		}
	default:
		err = errors.New("unsupported paused stage")
	}
	if err != nil {
		r.Pause = p
		return err
	}
	r.Events = append(r.Events, Event{Kind: "resume", Actor: actor, Reason: p.Stage + " after backend " + p.Reason})
	return nil
}

func failureReason(f *BackendFailure) string {
	if f == nil {
		return ""
	}
	return f.Reason
}

func failureMessage(f *BackendFailure) string {
	if f == nil {
		return ""
	}
	return f.Message
}

func refundRepair(r *Run, x *Repair) {
	switch x.Budget {
	case "gate":
		r.Pipeline.RepairUsed--
	case "final-fix":
		r.Pipeline.FinalFix.Repair = 0
	case "repair":
		r.RepairUsed--
	}
}

func pauseStage(r *Run, st *StageResult, kind, role string, config ReviewerConfig, execution string, f *BackendFailure) {
	p := r.Pipeline
	st.Status, st.Reason = "paused", "paused: backend "+f.Reason+" ("+f.Message+")"
	p.Status, p.Failed, p.Reason = "paused", st.Name, st.Reason
	pauseRun(r, st.Name, kind, role, config, execution, f, time.Now())
}

func readmitBlocker(r *Run, old Repair) (FixerConfig, string, error) {
	settings := r.Configs[len(r.Configs)-1].Settings
	cfg, err := admitFixer(settings.FixerBackend, settings.FixerModel)
	if err != nil {
		return cfg, "", err
	}
	fixerDigest, err := executableDigest(cfg.Executable)
	if err != nil {
		return cfg, "", err
	}
	switch old.Budget {
	case "gate":
		if r.Pipeline.RepairUsed >= r.Pipeline.RepairLimit {
			return cfg, "", fmt.Errorf("gate repair budget exhausted (%d/%d)", r.Pipeline.RepairUsed, r.Pipeline.RepairLimit)
		}
	case "final-fix":
	default:
		if r.ReviewUsed+r.RepairUsed >= settings.Limit {
			return cfg, "", errors.New("combined review and repair budget exhausted; increase the limit")
		}
	}
	return cfg, fixerDigest, nil
}

func readmitRepair(r *Run, old Repair) (*Repair, error) {
	cfg, fixerDigest, err := readmitBlocker(r, old)
	if err != nil {
		return nil, err
	}
	prompt := fixerPrompt
	if old.Stage != "" {
		prompt = gateFixerPrompt
	}
	x := old
	x.ID, x.Config, x.ConfigDigest, x.ExecutableDigest, x.Args = opaque(), cfg, digest(cfg), fixerDigest, fixerArgsFor(cfg, prompt)
	x.Status, x.Writer, x.Log, x.Head, x.Parent, x.Diff, x.Checks = "admitted", "not-started", "", "", "", "", nil
	x.Failure, x.Partial, x.PartialDigest, x.PartialBytes, x.Resumed, x.CostUSD, x.Tokens, x.Authority = nil, "", "", 0, true, 0, 0, ""
	switch x.Budget {
	case "gate":
		r.Pipeline.RepairUsed++
	case "final-fix":
		r.Pipeline.FinalFix.Repair = 1
	default:
		r.RepairUsed++
	}
	return &x, nil
}

func resumePipeline(s *Store, r *Run, p *BackendPause) error {
	pipe := r.Pipeline
	st := pipe.stage(p.Stage)
	if st == nil {
		return errors.New("paused stage is unknown")
	}
	if p.Kind == "review" {
		st.Status, st.Inputs = "stale", ""
		return admitPipeline(s, r)
	}
	if custodyHeld(s, r) {
		return errors.New("repository candidate custody is already held")
	}
	if len(pipe.Repairs) == 0 || pipe.Repairs[len(pipe.Repairs)-1].ID != p.Execution {
		return errors.New("paused repair is not the latest repair")
	}
	x, err := readmitRepair(r, pipe.Repairs[len(pipe.Repairs)-1])
	if err != nil {
		return err
	}
	pipe.Repairs = append(pipe.Repairs, *x)
	pipe.Active = &Step{ID: x.ID, Stage: st.Name, Kind: p.Kind}
	st.Status, st.Reason = "running", ""
	pipe.Status, pipe.Failed, pipe.Reason = "running", "", ""
	r.Status, r.Stage = "running", st.Name+"-repair"
	return nil
}

func autoResumeTime(r *Run) (time.Time, bool) {
	if r.Pause == nil || r.Pause.AutoResumeAt == "" || r.Status != "paused" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, r.Pause.AutoResumeAt)
	return at, err == nil
}

func untilAutoResume(s *Store, now time.Time) time.Duration {
	next := time.Hour
	for _, r := range s.Runs {
		if at, ok := autoResumeTime(r); ok && at.Sub(now) < next {
			next = at.Sub(now)
		}
	}
	return max(next, 0)
}

func fireAutoResumes(s *Store, now time.Time) bool {
	changed := false
	for _, r := range s.Runs {
		at, ok := autoResumeTime(r)
		if !ok || at.After(now) {
			continue
		}
		p, actor := r.Pause, autoActor(r)
		if p.AutoAction == "fallback" && p.AutoTo != nil {
			if err := switchBackend(r, p.Role, *p.AutoTo, actor, "fallback after "+p.Backend+" "+p.Reason); err != nil {
				p.AutoResumeAt, p.AutoAction, p.AutoTo, p.AutoNote = "", "", nil, "Automatic fallback failed: "+err.Error()
				r.Token, changed = opaque(), true
				continue
			}
		}
		r.AutoRetries++
		if err := resumeRun(s, r, actor); err != nil {
			p.AutoResumeAt, p.AutoAction, p.AutoTo, p.AutoNote = "", "", nil, "Automatic resume failed: "+err.Error()
		}
		r.Token, changed = opaque(), true
	}
	return changed
}

func switchBackend(r *Run, role string, to BackendChoice, actor, reason string) error {
	old := r.Configs[len(r.Configs)-1]
	settings := old.Settings
	from := settings.ReviewerBackend + " " + settings.ReviewerModel
	if role == "fixer" {
		from = settings.FixerBackend + " " + settings.FixerModel
		settings.FixerBackend, settings.FixerModel = to.Backend, to.Model
	} else {
		cfg, err := trustedAgent(to.Backend, to.Model)
		if err != nil {
			return err
		}
		settings.ReviewerBackend, settings.ReviewerModel = to.Backend, to.Model
		if r.Reviewer != nil {
			r.Reviewer = &cfg
		}
	}
	r.Configs = append(r.Configs, nextSnapshot(r.Configs[len(r.Configs)-1], settings, "fallback"))
	r.RuleSnapshots = append(r.RuleSnapshots, resolveRules(r))
	invalidate(r, "")
	supersede(r)
	r.Events = append(r.Events, Event{Kind: "backend-switch", Actor: actor, Reason: role + ": " + reason, From: from, To: to.Backend + " " + to.Model})
	return nil
}

func prepareRun(r *Run, intent, requested string) error {
	target, err := committedTarget(r.Submitted.Worktree, requested)
	if err != nil {
		return err
	}
	diff, err := gitRead(r.Submitted.Worktree, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", "--no-color", "--no-relative", "--src-prefix=a/", "--dst-prefix=b/", "--binary", target, r.Candidate, "--")
	if err != nil {
		return errors.New("cannot capture complete bounded diff")
	}
	settings := r.Configs[len(r.Configs)-1].Settings
	cfg, err := trustedAgent(settings.ReviewerBackend, settings.ReviewerModel)
	if err != nil {
		return err
	}
	if err = preflightPi(cfg); err != nil {
		return err
	}
	r.Pipeline = nil
	if settings.Preset == "full" {
		plan, digests, err := trustedPlan()
		if err != nil {
			return err
		}
		r.Pipeline = newPipeline(plan, digests)
		inherit(r)
	}
	r.Reviewer = &cfg
	r.SelectedDiff = string(diff)
	r.Intent, r.Target = intent, target
	return nil
}

func rerunStage(s *Store, r *Run, actor string) error {
	if p := r.Pipeline; p != nil {
		if d := p.open(); d != nil {
			return errors.New("answer the open " + d.Kind + " decision first")
		}
		if p.Status != "cancelled" && p.Status != "needs-decision" {
			return errors.New("nothing to rerun: the current stage is not paused, failed or cancelled")
		}
		var st *StageResult
		for i := range p.Stages {
			if c := &p.Stages[i]; c.Name == p.Failed || st == nil && c.Status == "cancelled" {
				st = c
			}
		}
		if st == nil {
			return errors.New("no stopped stage to rerun")
		}
		st.Status, st.Inputs = "stale", ""
		if err := admitPipeline(s, r); err != nil {
			return err
		}
		r.Events = append(r.Events, Event{Kind: "rerun", Actor: actor, Reason: "rerun from " + st.Name})
		return nil
	}
	if r.Repair != nil {
		return rerunRepair(r, actor)
	}
	if r.Review == nil || r.Status != "cancelled" && r.Status != "inconclusive" && r.Status != "unconfigured" && r.Status != "recovery-discarded" {
		return errors.New("nothing to rerun: the review is not paused, failed or cancelled")
	}
	if err := admitReviewAgain(r); err != nil {
		return err
	}
	r.Events = append(r.Events, Event{Kind: "rerun", Actor: actor, Reason: "rerun independent review"})
	return nil
}

func restartRun(s *Store, r *Run, source Identity, actor string) *Run {
	settings := r.Configs[len(r.Configs)-1].Settings
	next := &Run{Findings: append([]Finding(nil), r.Findings...), Predecessor: r.ID, ID: opaque(), Input: opaque(), Token: opaque(), Submitted: source, Candidate: source.Head, Stage: "intent", Status: "unconfigured", Configs: []Snapshot{nextSnapshot(r.Configs[len(r.Configs)-1], settings, "rerun from start of "+r.ID)}, Events: []Event{{Kind: "rerun-from-start", Actor: actor, Reason: "linked successor of " + r.ID}}}
	if r.Intent != "" && r.Target != "" {
		if err := prepareRun(next, r.Intent, r.Target); err != nil {
			next.Events = append(next.Events, Event{Kind: "prepare-needed", Actor: "daemon", Reason: err.Error()})
		}
	}
	r.Successor, r.Status, r.Token = next.ID, "restarted", opaque()
	if r.Pipeline != nil {
		r.Pipeline.Status = "restarted"
	}
	r.Events = append(r.Events, Event{Kind: "rerun-from-start", Actor: actor, Reason: "successor " + next.ID})
	s.Runs[next.ID] = next
	return next
}

func rerunnable(r *Run) bool {
	if p := r.Pipeline; p != nil {
		return p.open() == nil && (p.Status == "cancelled" || p.Status == "needs-decision" && p.Failed != "")
	}
	if r.Repair != nil {
		return stoppedRepair(r.Repair)
	}
	return r.Review != nil && (r.Status == "cancelled" || r.Status == "inconclusive" || r.Status == "unconfigured" || r.Status == "recovery-discarded")
}

var stoppedRepairStatuses = map[string]bool{"cancelled": true, "fixer-failed": true, "failed-check": true, "no-change": true, "scope-refused": true, "refused": true, "writer-conflict": true, "authority-changed": true, "unconfigured": true, "failed": true, "recovery-discarded": true}

func stoppedRepair(x *Repair) bool {
	return x.Stage == "" && stoppedRepairStatuses[x.Status] && (x.Writer == "stopped" || x.Writer == "not-started")
}

func rerunRepair(r *Run, actor string) error {
	old := r.Repair
	if !stoppedRepair(old) {
		return errors.New("nothing to rerun: the selected repair is not failed, cancelled or refused with a stopped writer")
	}
	if _, _, err := readmitBlocker(r, *old); err != nil {
		return err
	}
	_, statErr := os.Stat(old.Worktree)
	if statErr == nil {
		id, err := resolve(old.Worktree)
		if err != nil || id.Branch != old.Ref {
			return errors.New("candidate branch changed after the repair stopped; recovery required")
		}
		if _, err := gitRead(old.Worktree, "merge-base", "--is-ancestor", old.Base, id.Head); err != nil {
			return errors.New("candidate no longer descends from the repair base; recovery required")
		}
		if old.PartialDigest == "" {
			capturePartial(old)
		}
		for i := range r.Attempts {
			if r.Attempts[i].Execution == old.ID {
				r.Attempts[i].Partial = old.Partial
			}
		}
		if err := restoreCandidate(old); err != nil {
			return fmt.Errorf("cannot restore the private candidate: %w", err)
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	x, err := readmitRepair(r, *old)
	if err != nil {
		return err
	}
	x.Resumed = statErr == nil
	r.PriorRepairs = append(r.PriorRepairs, *old)
	r.Repair = x
	r.Status, r.Stage = "writing", "selected-repair"
	r.Events = append(r.Events, Event{Kind: "rerun", Actor: actor, Reason: "rerun selected repair after " + old.Status})
	return nil
}

func autoResumeLabel(s Settings) string {
	switch {
	case s.Mode == "ask":
		return "off (always off in ask mode)"
	case s.AutoResume == "off":
		return "off"
	case s.AutoResume == "on":
		return "on"
	}
	return "on (default in " + s.Mode + " mode)"
}

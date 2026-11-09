package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Recovery struct {
	ID            string             `json:"id"`
	Kind          string             `json:"kind"`
	Stage         string             `json:"stage,omitempty"`
	Execution     string             `json:"execution,omitempty"`
	Reason        string             `json:"reason"`
	Executors     []executorEvidence `json:"executors,omitempty"`
	Candidate     *CandidateEvidence `json:"candidate,omitempty"`
	Adopt         string             `json:"adopt,omitempty"`
	Options       []string           `json:"options"`
	Partial       string             `json:"partial,omitempty"`
	PartialDigest string             `json:"partial_digest,omitempty"`
	ProbedAt      string             `json:"probed_at,omitempty"`
	Answer        *Answer            `json:"answer,omitempty"`
	Outcome       string             `json:"outcome,omitempty"`
}

func requireRecovery(r *Run, kind, stage, execution, reason string) {
	r.Recovery = &Recovery{ID: opaque(), Kind: kind, Stage: stage, Execution: execution, Reason: reason, Options: []string{"inspect"}}
	r.Status = "recovery-required"
	r.Token = opaque()
	r.Events = append(r.Events, Event{Kind: "recovery-required", Actor: "daemon", Reason: kind + ": " + reason})
}

func closeRecovery(r *Run, kind string, a Answer, outcome string) {
	rec := r.Recovery
	rec.Answer, rec.Outcome = &a, outcome
	r.Recoveries = append(r.Recoveries, *rec)
	r.Recovery = nil
	r.Events = append(r.Events, Event{Kind: kind, Actor: a.Actor, Reason: outcome + ": " + a.Reason})
}

func reconcileRecoveries(s *Store) (changed bool) {
	for _, r := range s.Runs {
		if r.Recovery == nil {
			continue
		}
		observed := observeExecutors(r.Recovery.Execution)
		if adoptRecorded(r, observed) {
			changed = true
			continue
		}
		before := digest(r.Recovery)
		probeRecovery(r, observed)
		changed = changed || digest(r.Recovery) != before
	}
	return changed
}

func needsProbe(s *Store) bool {
	for _, r := range s.Runs {
		if rec := r.Recovery; rec != nil {
			if rec.ProbedAt == "" {
				return true
			}
			for _, e := range rec.Executors {
				if e.State == "running" {
					return true
				}
			}
		}
	}
	return false
}

func outputBuffer(b []byte, limit int) *boundedBuffer {
	out := &boundedBuffer{limit: limit}
	out.Write(b)
	return out
}

func adoptRecorded(r *Run, observed []observedExecutor) bool {
	rec := r.Recovery
	var adopted string
	switch rec.Kind {
	case "review":
		x := r.Review
		if x == nil || x.ID != rec.Execution || len(observed) != 1 || observed[0].State != "exited" || observed[0].record.InputDigest != x.InputDigest {
			return false
		}
		o := observed[0]
		var runErr error
		if o.Exit != 0 {
			runErr = fmt.Errorf("exit status %d", o.Exit)
		}
		done := reviewFrom(x, o.Exit, outputBuffer(o.stdout, executorStdoutLimit), outputBuffer(o.stderr, executorStderrLimit), runErr, false)
		recordReviewEvidence(x, "unverified: the controller restarted before the snapshot check", nil)
		r.Status = "running"
		if r.Pipeline == nil {
			finishReview(r, done)
		} else {
			r.Pipeline.Active = &Step{ID: x.ID, Stage: rec.Stage, Kind: "review"}
			if completeStep(r, stepOutcome{Run: r.ID, Step: x.ID, Status: done.Status, Log: done.Log, Result: done.Result, Failure: done.Failure, Answer: done.Answer}) != nil {
				return false
			}
		}
		adopted = "reviewer verdict " + x.Status + " from its complete executor record"
	case "commands":
		p := r.Pipeline
		if p == nil || p.Worktree == "" {
			return false
		}
		result, ok := recordedCommands(p, rec.Stage, observed)
		if !ok || worktreeClean(p.Worktree, r.Candidate) != nil {
			return false
		}
		p.Active = &Step{ID: rec.Execution, Stage: rec.Stage, Kind: "commands"}
		r.Status = "running"
		if completeStep(r, stepOutcome{Run: r.ID, Step: rec.Execution, Stage: result}) != nil {
			return false
		}
		adopted = rec.Stage + " " + result.Status + " from complete executor records"
	default:
		return false
	}
	rec.Executors = evidenceOf(observed)
	closeRecovery(r, "reconciled", Answer{Choice: "adopt", Actor: "daemon", Reason: "controller restart reconciliation"}, adopted)
	if p := r.Pipeline; p != nil && r.Status == "running" {
		p.Status, p.Failed, p.Reason = "parked", "", "controller restarted; adopted "+adopted+"; continue with pipeline"
		r.Status = "parked"
	}
	r.Token = opaque()
	return true
}

func recordedCommands(p *Pipeline, stage string, observed []observedExecutor) (StageResult, bool) {
	out := StageResult{Name: stage, Status: "passed", Reason: "all configured commands exited 0"}
	commands := p.commands(stage)
	for i, args := range commands {
		if i >= len(observed) || observed[i].State != "exited" {
			return out, false
		}
		o := observed[i]
		tail := &tailBuffer{limit: 16384}
		tail.Write(o.stdout)
		tail.Write(o.stderr)
		tail.total = int(o.record.Stdout + o.record.Stderr)
		res := CheckResult{Args: args, ExecutableDigest: p.CommandDigests[args[0]], Exit: o.Exit, Output: tail.String(), Status: "passed"}
		res.Log = fmt.Sprintf("exit=%d output=%d bytes truncated=%t", o.Exit, tail.total, tail.truncated())
		out.Commands = append(out.Commands, res)
		if o.Exit != 0 {
			res.Status = "failed"
			out.Commands[i] = res
			out.Status, out.Reason = "failed", fmt.Sprintf("%s exited %d", strings.Join(displayArgs(args), " "), o.Exit)
			return out, len(observed) == i+1
		}
	}
	return out, len(observed) == len(commands)
}

func evidenceOf(observed []observedExecutor) []executorEvidence {
	var out []executorEvidence
	for _, o := range observed {
		out = append(out, o.executorEvidence)
	}
	return out
}

func probeRecovery(r *Run, observed []observedExecutor) {
	rec := r.Recovery
	before := digest([]any{rec.Executors, rec.Candidate, rec.Adopt, rec.Options})
	rec.Executors = evidenceOf(observed)
	rec.Candidate = observeCandidate(r)
	rec.Options = []string{"inspect"}
	rec.Adopt = ""
	if stopped(rec) {
		if desc, _ := adoption(r); desc != "" {
			rec.Adopt = desc
			rec.Options = append(rec.Options, "adopt")
		}
		if discardable(r) == nil {
			rec.Options = append(rec.Options, "discard")
		}
		rec.Options = append(rec.Options, "new-run")
	}
	if rec.ProbedAt == "" || digest([]any{rec.Executors, rec.Candidate, rec.Adopt, rec.Options}) != before {
		rec.ProbedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

func stopped(rec *Recovery) bool {
	for _, e := range rec.Executors {
		if e.State == "running" {
			return false
		}
	}
	return true
}

func latestRepair(r *Run) *Repair {
	if r.Pipeline == nil {
		return r.Repair
	}
	if n := len(r.Pipeline.Repairs); n > 0 {
		return &r.Pipeline.Repairs[n-1]
	}
	return nil
}

func recoveryCandidate(r *Run) (worktree, ref, durable string) {
	rec := r.Recovery
	switch {
	case rec.Kind == "handback":
		return r.Submitted.Worktree, r.Submitted.Branch, r.Submitted.Head
	case rec.Kind == "repair" || rec.Kind == "conflict":
		if x := latestRepair(r); x != nil && x.ID == rec.Execution {
			return x.Worktree, x.Ref, x.Base
		}
	case r.Pipeline != nil && r.Pipeline.Worktree != "":
		return r.Pipeline.Worktree, r.Pipeline.Ref, r.Candidate
	case rec.Kind == "candidate":
		if h, err := home(); err == nil {
			return filepath.Join(h, "candidates", r.ID), "refs/heads/greenhouse/" + r.ID, r.Candidate
		}
	}
	return "", "", ""
}

func observeCandidate(r *Run) *CandidateEvidence {
	worktree, ref, durable := recoveryCandidate(r)
	if worktree == "" {
		return nil
	}
	c := &CandidateEvidence{Worktree: worktree, Ref: ref, Durable: durable}
	if _, err := os.Stat(worktree); os.IsNotExist(err) {
		c.Summary = "candidate worktree does not exist"
		if _, err := gitRead(r.Submitted.Worktree, "rev-parse", "-q", "--verify", ref); err != nil {
			c.Summary += " and its ref is absent"
		}
		return c
	}
	id, err := resolve(worktree)
	if err != nil {
		c.Summary = "candidate worktree is unreadable: " + err.Error()
		return c
	}
	c.Observed = id.Head
	if id.Branch != ref {
		c.Summary = "candidate worktree is on " + id.Branch + ", not " + ref
		return c
	}
	status, err := gitRead(worktree, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	c.Clean = err == nil && len(status) == 0
	_, mergeErr := gitRead(worktree, "rev-parse", "-q", "--verify", "MERGE_HEAD")
	c.Merging = mergeErr == nil
	switch {
	case c.Merging:
		c.Summary = "a merge is in progress"
	case !c.Clean:
		c.Summary = "the worktree has uncommitted changes"
	case c.Observed == durable:
		c.Summary = "unchanged at the recorded head"
	default:
		c.Summary = "advanced past the recorded head"
	}
	return c
}

func parents(dir, head string) []string {
	b, err := gitRead(dir, "rev-list", "--parents", "-n", "1", head)
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return nil
	}
	return fields[1:]
}

func adoption(r *Run) (string, func() error) {
	rec, c := r.Recovery, r.Recovery.Candidate
	if rec.Kind == "handback" {
		source, err := resolve(r.Submitted.Worktree)
		if err != nil || source.Branch != r.Submitted.Branch {
			return "", nil
		}
		switch source.Head {
		case r.Candidate:
			return "source branch already points to the candidate; record the handback", func() error {
				r.Pipeline.Handback, r.Pipeline.HandbackError = "handed-back", ""
				return nil
			}
		case r.Submitted.Head:
			return "source branch is unchanged; return the candidate to awaiting handback", func() error {
				r.Pipeline.Handback, r.Pipeline.HandbackError = "awaiting-handback", ""
				return nil
			}
		}
		return "", nil
	}
	if c == nil || c.Observed == "" || !c.Clean || c.Merging {
		return "", nil
	}
	switch rec.Kind {
	case "repair", "conflict":
		x := latestRepair(r)
		got := parents(c.Worktree, c.Observed)
		if x == nil || c.Observed == x.Base {
			return "", nil
		}
		if rec.Kind == "conflict" {
			if len(got) != 2 || got[0] != x.Base || got[1] != r.Target || conflictMarkers(c.Worktree, x.Paths) != nil {
				return "", nil
			}
		} else {
			diff, err := gitRead(c.Worktree, "diff", "--no-ext-diff", "--no-textconv", "--binary", x.Base, c.Observed, "--")
			if len(got) != 1 || got[0] != x.Base || x.Diff == "" || err != nil || string(diff) != x.Diff {
				return "", nil
			}
			for _, check := range x.Checks {
				if check.Status != "passed" {
					return "", nil
				}
			}
			if x.Stage == "" && len(x.Checks) != len(x.Config.Checks) {
				return "", nil
			}
		}
		head := c.Observed
		return "candidate commit " + short(head) + " matches the recorded repair; adopt it as fixed, not independently rereviewed", func() error {
			done := *x
			done.Head, done.Parent, done.Writer = head, x.Base, "stopped"
			done.Status = "fixed-not-independently-rereviewed"
			done.Handback = "awaiting-handback"
			if r.Pipeline != nil {
				done.Handback = "pipeline-custody"
				r.Pipeline.Active = &Step{ID: x.ID, Stage: rec.Stage, Kind: rec.Kind}
				r.Status = "running"
				return completeStep(r, stepOutcome{Run: r.ID, Step: x.ID, Repair: &done})
			}
			*x = done
			r.Status, r.Candidate = x.Status, head
			return nil
		}
	case "sync":
		got := parents(c.Worktree, c.Observed)
		if c.Observed == r.Candidate || len(got) != 2 || got[0] != r.Candidate || got[1] != r.Target {
			return "", nil
		}
		head := c.Observed
		return "candidate merge " + short(head) + " has the recorded candidate and target as parents; adopt the synchronized head", func() error {
			r.Pipeline.Active = &Step{ID: rec.Execution, Stage: "sync", Kind: "sync"}
			r.Status = "running"
			return completeStep(r, stepOutcome{Run: r.ID, Step: rec.Execution, Head: head, Stage: StageResult{Name: "sync", Status: "passed", Reason: "merged target without rebase; adopted after recovery"}})
		}
	case "candidate":
		if c.Observed != r.Candidate {
			return "", nil
		}
		return "private candidate worktree exists at the recorded head; adopt it and continue", func() error {
			r.Pipeline.Worktree, r.Pipeline.Ref = c.Worktree, c.Ref
			return nil
		}
	}
	return "", nil
}

func discardable(r *Run) error {
	rec := r.Recovery
	switch rec.Kind {
	case "handback", "run":
		return fmt.Errorf("a %s outcome has no stage to rerun", rec.Kind)
	case "candidate":
		if c := rec.Candidate; c == nil || c.Observed != "" || !strings.Contains(c.Summary, "ref is absent") {
			return errors.New("a partially created candidate cannot be discarded; adopt it or start a linked new run")
		}
	}
	if c := rec.Candidate; c != nil && c.Observed != "" && rec.Kind != "candidate" {
		if _, err := gitRead(c.Worktree, "merge-base", "--is-ancestor", c.Durable, c.Observed); err != nil {
			return errors.New("candidate no longer descends from the recorded head; start a linked new run")
		}
	}
	return nil
}

func recoverRun(s *Store, r *Run, source Identity, a Answer) (*Run, error) {
	rec := r.Recovery
	if rec == nil {
		return nil, errors.New("no open recovery")
	}
	if !validText(a.Reason) {
		return nil, errors.New("reason required")
	}
	probeRecovery(r, observeExecutors(rec.Execution))
	if a.Choice == "inspect" {
		r.Events = append(r.Events, Event{Kind: "recovery-inspect", Actor: a.Actor, Reason: a.Reason})
		r.Token = opaque()
		return r, nil
	}
	if !has(rec.Options, a.Choice) {
		if !stopped(rec) {
			return nil, errors.New("an orphaned executor still holds its lease; inspect again after it exits")
		}
		if err := discardable(r); a.Choice == "discard" && err != nil {
			return nil, err
		}
		return nil, errors.New("choice not offered by the current recovery evidence: " + a.Choice)
	}
	if a.Choice != "new-run" && source != r.Submitted && rec.Kind != "handback" {
		return nil, errors.New("source head or branch moved; start a linked new run")
	}
	if c := rec.Candidate; a.Choice != "new-run" && c != nil && c.Observed != "" && rec.Kind != "handback" {
		if err := claimLease(r); err != nil {
			return nil, err
		}
	}
	switch a.Choice {
	case "adopt":
		desc, apply := adoption(r)
		if err := apply(); err != nil {
			return nil, err
		}
		if p := r.Pipeline; p != nil && rec.Kind == "handback" {
			p.Status, p.Failed, p.Reason = "ci-ready", "", ""
			if localOnlyPipeline(p) {
				p.Status = "locally-ready"
			}
			r.Status = p.Status
		} else if p != nil {
			if st := p.stage(rec.Stage); st != nil && st.Status == "unknown-outcome" {
				st.Status, st.Inputs = "stale", ""
			}
			desc += continuePipeline(s, r, rec.Stage)
		}
		closeRecovery(r, "recovery-adopt", a, desc)
	case "discard":
		if err := discardStep(r); err != nil {
			return nil, err
		}
		outcome := "discarded the interrupted " + rec.Kind + " and reran " + rec.Stage
		if err := rerunAfterDiscard(s, r, a.Actor); err != nil {
			outcome = "discarded the interrupted " + rec.Kind + "; rerun refused: " + err.Error()
		}
		closeRecovery(r, "recovery-discard", a, outcome)
	case "new-run":
		closeRecovery(r, "recovery-new-run", a, "kept this run's evidence and refs; linked a new run")
		if r.Repair != nil && r.Repair.Handback != "handed-back" {
			r.Repair.Handback = "released"
		}
		next := restartRun(s, r, source, a.Actor)
		next.ReviewUsed, next.RepairUsed = r.ReviewUsed, r.RepairUsed
		if p := r.Pipeline; p != nil {
			next.Inherited = &Inheritance{From: r.ID, GateRepairs: p.RepairUsed, FinalFix: p.FinalFix}
		} else {
			next.Inherited = &Inheritance{From: r.ID}
		}
		next.Inherited.Review, next.Inherited.Repair = r.ReviewUsed, r.RepairUsed
		inherit(next)
		reason := "inherited consumed budget from " + r.ID
		if r.Pipeline != nil {
			reason += fmt.Sprintf("; gate repairs %d", r.Pipeline.RepairUsed)
		}
		next.Events = append(next.Events, Event{Kind: "recovery-new-run", Actor: a.Actor, Reason: reason, Used: r.ReviewUsed + r.RepairUsed, Review: r.ReviewUsed, Repair: r.RepairUsed})
		r.Status = "superseded"
		return next, nil
	}
	r.Token = opaque()
	return r, nil
}

func leaseEpochOf(r *Run) *int {
	if r.Pipeline != nil {
		return &r.Pipeline.LeaseEpoch
	}
	if r.Repair != nil {
		return &r.Repair.LeaseEpoch
	}
	return nil
}

func claimLease(r *Run) error {
	epoch, c := leaseEpochOf(r), r.Recovery.Candidate
	if epoch == nil {
		return nil
	}
	return withWriterLock(r.Submitted.Repository, func() error {
		if r.Recovery.Kind == "candidate" && *epoch == 0 {
			l, err := readLease(r.Submitted.Repository, c.Ref)
			if err != nil {
				return err
			}
			if l != nil {
				*epoch = l.Epoch
			}
		}
		return checkLease(r.Submitted.Repository, c.Ref, *epoch)
	})
}

func discardStep(r *Run) error {
	c := r.Recovery.Candidate
	if c == nil || c.Observed == "" {
		return nil
	}
	x := &Repair{Worktree: c.Worktree, Ref: c.Ref, Base: c.Durable}
	return withWriterLock(r.Submitted.Repository, func() error {
		if epoch := leaseEpochOf(r); epoch != nil {
			next, err := renewLease(r.Submitted.Repository, c.Ref, *epoch)
			if err != nil {
				return err
			}
			*epoch = next
			if y := latestRepair(r); y != nil {
				y.LeaseEpoch = next
			}
		}
		return restoreObserved(r, x)
	})
}

func restoreObserved(r *Run, x *Repair) error {
	rec, c := r.Recovery, r.Recovery.Candidate
	if c.Observed != c.Durable {
		if _, err := controllerGit(r.Submitted.Worktree, "update-ref", "refs/greenhouse/recovered/"+rec.ID, c.Observed); err != nil {
			return fmt.Errorf("cannot preserve the observed candidate: %w", err)
		}
	}
	if !c.Clean || c.Merging {
		capturePartial(x)
		rec.Partial, rec.PartialDigest = x.Partial, x.PartialDigest
	}
	if err := restoreCandidate(x); err != nil {
		return fmt.Errorf("cannot restore the private candidate: %w", err)
	}
	if y := latestRepair(r); y != nil && y.ID == rec.Execution {
		y.Status, y.Writer = "recovery-discarded", "stopped"
		if c.Observed != c.Durable {
			y.Log = "observed candidate preserved at refs/greenhouse/recovered/" + rec.ID
		}
	}
	return nil
}

func continuePipeline(s *Store, r *Run, stage string) string {
	p := r.Pipeline
	p.Failed, p.Reason = "", ""
	if err := admitPipeline(s, r); err != nil {
		p.Status, p.Failed, p.Reason = "needs-decision", stage, "recovered; continue refused: "+err.Error()
		r.Status, r.Stage = "needs-decision", stage
		return "; continue refused: " + err.Error()
	}
	return ""
}

func rerunAfterDiscard(s *Store, r *Run, actor string) error {
	rec := r.Recovery
	if p := r.Pipeline; p != nil {
		stage := rec.Stage
		if rec.Kind == "conflict" {
			stage = "sync"
		}
		if st := p.stage(stage); st != nil {
			st.Status, st.Inputs = "stale", ""
		}
		if refused := continuePipeline(s, r, stage); refused != "" {
			return errors.New(strings.TrimPrefix(refused, "; continue refused: "))
		}
		return nil
	}
	r.Status = "recovery-discarded"
	switch rec.Kind {
	case "review":
		r.Review.Status = "recovery-discarded"
		return admitReviewAgain(r)
	case "repair":
		old := *r.Repair
		x, err := readmitRepair(r, old)
		if err != nil {
			return err
		}
		_, statErr := os.Stat(old.Worktree)
		x.Resumed = statErr == nil
		r.PriorRepairs = append(r.PriorRepairs, old)
		r.Repair = x
		r.Status, r.Stage = "writing", "selected-repair"
		r.Events = append(r.Events, Event{Kind: "rerun", Actor: actor, Reason: "rerun selected repair after recovery discard"})
		return nil
	}
	return errors.New("nothing to rerun for " + rec.Kind)
}

type Inheritance struct {
	From        string    `json:"from"`
	Review      int       `json:"review"`
	Repair      int       `json:"repair"`
	GateRepairs int       `json:"gate_repairs"`
	FinalFix    *FinalFix `json:"final_fix,omitempty"`
}

func ownReviews(r *Run) int {
	if r.Inherited == nil {
		return r.ReviewUsed
	}
	return r.ReviewUsed - r.Inherited.Review
}

func inherit(r *Run) {
	if r.Inherited == nil || r.Pipeline == nil {
		return
	}
	r.Pipeline.RepairUsed = max(r.Pipeline.RepairUsed, r.Inherited.GateRepairs)
	if r.Pipeline.FinalFix == nil && r.Inherited.FinalFix != nil {
		spent := *r.Inherited.FinalFix
		r.Pipeline.FinalFix = &spent
	}
}

type CandidateEvidence struct {
	Worktree string `json:"worktree"`
	Ref      string `json:"ref"`
	Durable  string `json:"durable"`
	Observed string `json:"observed,omitempty"`
	Clean    bool   `json:"clean"`
	Merging  bool   `json:"merging"`
	Summary  string `json:"summary"`
}

func renderRecovery(w io.Writer, r *Run) {
	if r.Predecessor != "" {
		fmt.Fprintf(w, "Linked from run %s\n", r.Predecessor)
	}
	for _, old := range r.Recoveries {
		if old.Answer != nil {
			fmt.Fprintf(w, "Recovered %s: %s by %s: %s\n", old.Kind, old.Answer.Choice, old.Answer.Actor, old.Outcome)
		}
	}
	rec := r.Recovery
	if rec == nil {
		return
	}
	fmt.Fprintf(w, "RECOVERY REQUIRED: %s at %s\n%s\n", rec.Kind, rec.Stage, rec.Reason)
	if rec.ProbedAt == "" {
		fmt.Fprintln(w, "Evidence: not probed yet")
	}
	for _, e := range rec.Executors {
		fmt.Fprintf(w, "Executor %s: %s exit=%d %s\n", short(e.ID[:strings.LastIndex(e.ID, ".")])+e.ID[strings.LastIndex(e.ID, "."):], e.State, e.Exit, e.Detail)
	}
	if c := rec.Candidate; c != nil {
		observed := short(c.Observed)
		if observed == "" {
			observed = "none"
		}
		fmt.Fprintf(w, "Candidate %s: %s\n  recorded %s observed %s\n", strings.TrimPrefix(c.Ref, "refs/heads/"), c.Summary, short(c.Durable), observed)
	}
	if rec.Adopt != "" {
		fmt.Fprintln(w, "Adopt: "+rec.Adopt)
	}
	fmt.Fprintf(w, "Options: %s\n[e] answers recovery gate %s\n", strings.Join(rec.Options, ", "), short(rec.ID))
}

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type YoloPolicy struct {
	Actor string `json:"actor"`
	Cap   string `json:"cap,omitempty"`
}
type Answer struct {
	Choice   string   `json:"choice"`
	Actor    string   `json:"actor"`
	Reason   string   `json:"reason"`
	Action   string   `json:"action,omitempty"`
	Selected []string `json:"selected,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}
type Decision struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Stage     string   `json:"stage"`
	Category  string   `json:"category,omitempty"`
	UserOnly  bool     `json:"user_only"`
	Candidate string   `json:"candidate"`
	Binding   string   `json:"binding"`
	Findings  []string `json:"findings,omitempty"`
	Options   []string `json:"options"`
	Reason    string   `json:"reason"`
	Status    string   `json:"status"`
	Answer    *Answer  `json:"answer,omitempty"`
}
type Acceptance struct {
	Decision  string   `json:"decision"`
	Candidate string   `json:"candidate"`
	Inputs    string   `json:"inputs"`
	Config    string   `json:"config"`
	Findings  []string `json:"findings"`
	Actor     string   `json:"actor"`
	Reason    string   `json:"reason"`
}
type FinalFix struct {
	Decision      string `json:"decision"`
	Actor         string `json:"actor"`
	Reason        string `json:"reason"`
	Candidate     string `json:"candidate"`
	ReviewUsed    int    `json:"review_used"`
	RepairUsed    int    `json:"repair_used"`
	RepairAllowed bool   `json:"repair_allowed"`
	Repair        int    `json:"repair"`
	Review        int    `json:"review"`
}
type Resolution struct {
	Finding   string `json:"finding"`
	Repair    string `json:"repair"`
	Review    string `json:"review"`
	Candidate string `json:"candidate"`
	Reopened  string `json:"reopened,omitempty"`
}
type Budget struct {
	Review      int `json:"review"`
	Repair      int `json:"repair"`
	Total       int `json:"total"`
	Limit       int `json:"limit"`
	GateRetries int `json:"gate_retries"`
	GateLimit   int `json:"gate_limit"`
	FinalRepair int `json:"final_fix_repair"`
	FinalReview int `json:"final_fix_review"`
}

var authorityCategories = map[string]bool{"intent": true, "scope": true, "acceptance": true, "credentials": true, "permissions": true, "destructive": true}

type authorityError struct{ category, reason string }

func (e authorityError) Error() string { return e.category + ": " + e.reason }

func has(list []string, want string) bool {
	for _, x := range list {
		if x == want {
			return true
		}
	}
	return false
}

func (p *Pipeline) open() *Decision {
	for i := range p.Decisions {
		if p.Decisions[i].Status == "open" {
			return &p.Decisions[i]
		}
	}
	return nil
}

func budgetOf(r *Run) *Budget {
	b := &Budget{Review: r.ReviewUsed, Repair: r.RepairUsed, Total: r.ReviewUsed + r.RepairUsed, Limit: r.Configs[len(r.Configs)-1].Settings.Limit}
	if p := r.Pipeline; p != nil {
		b.GateRetries, b.GateLimit = p.RepairUsed, p.RepairLimit
		if g := p.FinalFix; g != nil {
			b.FinalRepair, b.FinalReview = g.Repair, g.Review
		}
	}
	return b
}

func outstanding(r *Run) (actionable, informational []Finding) {
	resolved := map[string]bool{}
	if r.Pipeline != nil {
		for _, x := range r.Pipeline.Resolutions {
			resolved[x.Finding] = resolved[x.Finding] || x.Reopened == ""
		}
	}
	for _, f := range r.Findings {
		switch {
		case resolved[f.ID]:
		case f.Priority == "P3" || !validText(f.Action):
			informational = append(informational, f)
		default:
			actionable = append(actionable, f)
		}
	}
	return actionable, informational
}

func repairableFindings(r *Run, findings []Finding) []Finding {
	if _, err := trustedFixer(); err != nil {
		return nil
	}
	paths, err := changedPaths(r)
	if err != nil {
		return nil
	}
	changed := map[string]bool{}
	for _, name := range paths {
		changed[name] = true
	}
	var out []Finding
	for _, f := range findings {
		if changed[f.File] {
			out = append(out, f)
		}
	}
	return out
}

func findingIDs(findings []Finding) []string {
	var ids []string
	for _, f := range findings {
		ids = append(ids, f.ID)
	}
	return ids
}

func recordReview(r *Run, x *Execution) {
	p := r.Pipeline
	index := map[string]int{}
	for i, f := range r.Findings {
		index[f.ID] = i
	}
	reported := map[string]bool{}
	for _, f := range x.Result.Findings {
		reported[f.ID] = true
		if i, ok := index[f.ID]; ok {
			r.Findings[i] = f
		} else {
			r.Findings = append(r.Findings, f)
		}
	}
	for i := range p.Resolutions {
		if res := &p.Resolutions[i]; res.Reopened == "" && reported[res.Finding] {
			res.Reopened = x.ID
		}
	}
	actionable, informational := outstanding(r)
	for _, f := range append(actionable, informational...) {
		if repair := repairedBy(p, f); !reported[f.ID] && repair != "" {
			p.Resolutions = append(p.Resolutions, Resolution{Finding: f.ID, Repair: repair, Review: x.ID, Candidate: r.Candidate})
		}
	}
}

func repairedBy(p *Pipeline, finding Finding) string {
	for _, repair := range p.Repairs {
		if repair.Stage != "review" || repair.Head == "" {
			continue
		}
		for _, f := range repair.Selected {
			if f.ID == finding.ID && f.Head == finding.Head {
				return repair.ID
			}
		}
	}
	return ""
}

func decisionBinding(r *Run, stage string) string {
	return digest([]any{r.Candidate, stageInputs(r, stage), r.Configs[len(r.Configs)-1].Settings.Limit})
}

func openDecision(r *Run, st *StageResult, status string, d Decision) {
	p := r.Pipeline
	d.ID, d.Status, d.Stage, d.Candidate, d.Binding = opaque(), "open", st.Name, r.Candidate, decisionBinding(r, st.Name)
	p.Decisions = append(p.Decisions, d)
	p.Gate = d.ID
	park(r, st, status, d.Reason)
	r.Events = append(r.Events, Event{Kind: "decision-open", Actor: "daemon", Reason: d.Kind + ": " + d.Reason})
}

func supersede(r *Run) {
	p := r.Pipeline
	if p == nil {
		return
	}
	if d := p.open(); d != nil && !d.UserOnly && d.Binding != decisionBinding(r, d.Stage) {
		d.Status = "superseded"
		if p.Gate == d.ID {
			p.Gate = ""
		}
	}
}

func capOptions(p *Pipeline) []string {
	options := []string{"accept-head"}
	if p.FinalFix == nil {
		options = append(options, "final-fix")
	}
	return append(options, "increase-cap", "rethink", "discard")
}

func yoloActor(r *Run) (string, bool) {
	if r.Configs[len(r.Configs)-1].Settings.Mode != "yolo" || r.Pipeline.Plan.Yolo == nil {
		return "", false
	}
	return "yolo:" + r.Pipeline.Plan.Yolo.Actor, true
}

func capDecision(s *Store, r *Run, st *StageResult, reason string) *Step {
	p := r.Pipeline
	actionable, _ := outstanding(r)
	b := budgetOf(r)
	reason = fmt.Sprintf("review cap reached (%d/%d: review %d + repair %d); %s", b.Total, b.Limit, b.Review, b.Repair, reason)
	openDecision(r, st, "needs-decision", Decision{Kind: "cap", Findings: findingIDs(actionable), Options: capOptions(p), Reason: reason})
	if actor, ok := yoloActor(r); ok && p.Plan.Yolo.Cap != "" && has(p.open().Options, p.Plan.Yolo.Cap) {
		if _, err := applyChoice(s, r, p.open(), Answer{Choice: p.Plan.Yolo.Cap, Actor: actor, Reason: "trusted pipeline.json yolo cap policy"}); err == nil {
			return advancePipeline(s, r)
		}
	}
	return nil
}

func findingsNext(s *Store, r *Run, st *StageResult) *Step {
	p := r.Pipeline
	actionable, _ := outstanding(r)
	repairable := repairableFindings(r, actionable)
	b := budgetOf(r)
	if g := p.FinalFix; g != nil && g.RepairAllowed && g.Repair == 0 && len(repairable) > 0 {
		if err := admitGateRepair(r, st, g.Actor, repairable); err == nil {
			return p.Active
		}
	}
	if b.Total+2 > b.Limit {
		return capDecision(s, r, st, fmt.Sprintf("%d actionable findings remain and no room for a repair and its rereview", len(actionable)))
	}
	options := []string{"repair", "rethink", "discard"}
	reason := fmt.Sprintf("%d actionable findings await authorization", len(actionable))
	if len(repairable) == 0 {
		options = options[1:]
		reason = fmt.Sprintf("%d actionable findings need the author: no trusted fixer or no finding in a changed path", len(actionable))
	} else if mode := r.Configs[len(r.Configs)-1].Settings.Mode; mode != "ask" {
		actor := "auto"
		if mode == "yolo" {
			var ok bool
			if actor, ok = yoloActor(r); !ok {
				reason = "yolo mode has no configured actor in pipeline.json; " + reason
			}
		}
		if actor != "" {
			err := admitGateRepair(r, st, actor, repairable)
			if err == nil {
				return p.Active
			}
			reason = "repair refused: " + err.Error()
		}
	}
	openDecision(r, st, "needs-decision", Decision{Kind: "findings", Findings: findingIDs(actionable), Options: options, Reason: reason})
	return nil
}

func applyChoice(s *Store, r *Run, d *Decision, a Answer) (*Run, error) {
	p := r.Pipeline
	if !has(d.Options, a.Choice) {
		return nil, errors.New("choice not offered by the open decision: " + a.Choice)
	}
	if !validText(a.Reason) {
		return nil, errors.New("reason required")
	}
	if d.UserOnly && (a.Actor == "auto" || strings.HasPrefix(a.Actor, "yolo:")) {
		return nil, errors.New("user-only authority cannot be delegated")
	}
	st := p.stage(d.Stage)
	next := r
	resume := true
	switch a.Choice {
	case "repair":
		if d.Kind == "findings" {
			actionable, _ := outstanding(r)
			grouped := map[string]bool{}
			for _, id := range d.Findings {
				grouped[id] = true
			}
			var selected []Finding
			for _, f := range repairableFindings(r, actionable) {
				if grouped[f.ID] && (len(a.Selected) == 0 || has(a.Selected, f.ID)) {
					selected = append(selected, f)
				}
			}
			if len(selected) == 0 || len(a.Selected) > len(selected) {
				return nil, errors.New("select repairable finding IDs from the grouped request")
			}
			if err := admitGateRepair(r, st, a.Actor, selected); err != nil {
				return nil, err
			}
		} else {
			if p.RepairUsed >= p.RepairLimit {
				return nil, fmt.Errorf("gate repair budget exhausted (%d/%d)", p.RepairUsed, p.RepairLimit)
			}
			if err := admitGateRepair(r, st, a.Actor, nil); err != nil {
				return nil, err
			}
		}
	case "accept-head":
		actionable, _ := outstanding(r)
		review := p.stage("review")
		in := stageInputs(r, "review")
		p.Acceptances = append(p.Acceptances, Acceptance{Decision: d.ID, Candidate: r.Candidate, Inputs: in, Config: r.Configs[len(r.Configs)-1].ID, Findings: findingIDs(actionable), Actor: a.Actor, Reason: a.Reason})
		review.Status, review.Inputs, review.Candidate = "accepted-risk", in, r.Candidate
		review.Reason = fmt.Sprintf("review risk accepted by %s for this exact candidate and configuration (%d findings); not a clean pass", a.Actor, len(actionable))
	case "final-fix":
		actionable, _ := outstanding(r)
		g := &FinalFix{Decision: d.ID, Actor: a.Actor, Reason: a.Reason, Candidate: r.Candidate, ReviewUsed: r.ReviewUsed, RepairUsed: r.RepairUsed, RepairAllowed: len(repairableFindings(r, actionable)) > 0}
		p.FinalFix = g
		if !g.RepairAllowed {
			review := p.stage("review")
			review.Status, review.Inputs = "stale", ""
		}
	case "increase-cap":
		old := r.Configs[len(r.Configs)-1]
		if a.Limit <= old.Settings.Limit || a.Limit > 1000 {
			return nil, errors.New("increase-cap needs a limit above the current limit and at most 1000")
		}
		settings := old.Settings
		settings.Limit = a.Limit
		r.Configs = append(r.Configs, Snapshot{opaque(), settings, "cap-increase"})
		r.Events = append(r.Events, Event{Kind: "cap-increase", Actor: a.Actor, Reason: a.Reason, Used: r.ReviewUsed + r.RepairUsed, Before: old.Settings.Limit, After: a.Limit, Review: r.ReviewUsed, Repair: r.RepairUsed})
	case "rethink":
		resume = false
		p.Status, r.Status = "rethought", "rethought"
		next = &Run{Findings: append([]Finding(nil), r.Findings...), Predecessor: r.ID, ID: opaque(), Input: opaque(), Token: opaque(), Submitted: r.Submitted, Candidate: r.Submitted.Head, Stage: "intent", Status: "unconfigured", Configs: []Snapshot{{opaque(), r.Configs[len(r.Configs)-1].Settings, "rethink of " + r.ID}}, Events: []Event{{Kind: "rethink", Actor: a.Actor, Reason: "successor of " + r.ID + ": " + a.Reason, Used: r.ReviewUsed + r.RepairUsed, Review: r.ReviewUsed, Repair: r.RepairUsed}}}
		r.Successor = next.ID
		s.Runs[next.ID] = next
		s.Slots[digest([]string{r.Submitted.Repository, r.Submitted.Worktree})] = next.ID
	case "discard":
		resume = false
		p.Status, r.Status = "discarded", "discarded"
	default:
		return nil, errors.New("unsupported choice")
	}
	d.Status = "answered"
	d.Answer = &a
	if p.Gate == d.ID {
		p.Gate = ""
	}
	r.Events = append(r.Events, Event{Kind: "decision", Actor: a.Actor, Reason: a.Choice + ": " + a.Reason})
	if resume {
		p.Status, p.Failed, p.Reason = "running", "", ""
		r.Status, r.Stage = "running", "pipeline"
	}
	r.Token = opaque()
	return next, nil
}

func escalation(stdout []byte) (string, string) {
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	text := ""
	for scanner.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Stop    string `json:"stopReason"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &ev) != nil || ev.Type != "message_end" || ev.Message.Role != "assistant" || ev.Message.Stop != "stop" {
			continue
		}
		text = ""
		for _, c := range ev.Message.Content {
			if c.Type == "text" {
				text += c.Text
			}
		}
	}
	var answer struct {
		Escalate string `json:"escalate"`
		Reason   string `json:"reason"`
	}
	if _, err := exactFields([]byte(text), "escalate", "reason"); err != nil || json.Unmarshal([]byte(text), &answer) != nil || !authorityCategories[answer.Escalate] || !validText(answer.Reason) {
		return "", ""
	}
	return answer.Escalate, answer.Reason
}

func parseSelection(line string, group []string) ([]string, bool, error) {
	if strings.TrimSpace(line) == "" {
		return nil, true, nil
	}
	entries := strings.FieldsFunc(line, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	if len(entries) == 0 {
		return nil, false, errors.New("no finding ID entered; leave the line empty to select the whole group")
	}
	var ids []string
	for _, entry := range entries {
		var matches []string
		for _, id := range group {
			if strings.HasPrefix(id, entry) {
				matches = append(matches, id)
			}
		}
		switch {
		case len(matches) == 0:
			return nil, false, errors.New("unknown finding ID: " + entry)
		case len(matches) > 1:
			return nil, false, errors.New("ambiguous finding ID: " + entry)
		case has(ids, matches[0]):
			return nil, false, errors.New("duplicate finding ID: " + matches[0])
		}
		ids = append(ids, matches[0])
	}
	return ids, false, nil
}

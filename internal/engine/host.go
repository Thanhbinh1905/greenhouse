package engine

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

type Host struct {
	Name     string            `json:"name"`
	ID       string            `json:"id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type HostAcceptance struct {
	Candidate string `json:"candidate"`
	Review    string `json:"review"`
	Actor     string `json:"actor"`
	Reason    string `json:"reason"`
}

type CarriedFinding struct {
	Finding Finding `json:"finding"`
	Kept    bool    `json:"kept"`
	Reason  string  `json:"reason"`
}

type HostResult struct {
	Host             Host              `json:"host"`
	Grant            string            `json:"grant"`
	Review           string            `json:"review_id"`
	AcceptedHeads    []HostAcceptance  `json:"accepted_heads,omitempty"`
	Head             string            `json:"head"`
	Target           string            `json:"target"`
	Status           string            `json:"status"`
	Verdict          string            `json:"verdict,omitempty"`
	Current          bool              `json:"current"`
	Findings         []Finding         `json:"findings"`
	Dismissed        []Finding         `json:"dismissed_findings,omitempty"`
	Carried          []CarriedFinding  `json:"carried_findings,omitempty"`
	Resolutions      []ResolutionCheck `json:"resolutions,omitempty"`
	Conflicts        []FindingConflict `json:"conflicts,omitempty"`
	DismissalReasons map[string]string `json:"dismissal_reasons,omitempty"`
	Generated        []GeneratedPath   `json:"generated,omitempty"`
	Uncovered        []UncoveredPath   `json:"uncovered,omitempty"`
	Rules            string            `json:"rules_digest"`
	Config           string            `json:"config_digest"`
	Input            string            `json:"input_digest"`
	Backend          string            `json:"backend"`
	Model            string            `json:"model"`

	Reason           string     `json:"reason,omitempty"`
	Diagnostic       string     `json:"diagnostic,omitempty"`
	Usage            TokenUsage `json:"usage"`
	ReviewBase       string     `json:"review_base,omitempty"`
	ReviewBaseReason string     `json:"review_base_reason,omitempty"`
}

var hostOwned = map[string]string{
	"repair":           "author repair",
	"resolve-conflict": "author repair",
	"pipeline":         "publication and CI",
	"handback":         "candidate custody",
	"decide":           "cap decisions",
	"rerun-from-start": "cap decisions",
	"save":             "saved configuration",
	"trust":            "saved configuration",
	"migrate":          "saved configuration",
}

var hostActions = []string{"attach", "inspect", "apply", "resolve-rules", "prepare", "review", "accept-head", "cancel", "resume", "rerun", "recover", "new"}

func validHost(h *Host) bool {
	if h == nil || !ValidText(h.Name) || !ValidText(h.ID) || len(h.Name) > 128 || len(h.ID) > 256 || len(h.Metadata) > 16 {
		return false
	}
	for k, v := range h.Metadata {
		if !ValidText(k) || len(k) > 64 || len(v) > 256 || v != "" && !ValidText(v) {
			return false
		}
	}
	return true
}

func sameHost(a, b *Host) bool {
	return a != nil && b != nil && a.Name == b.Name && a.ID == b.ID
}

func HostLabel(h *Host) string {
	return h.Name + " " + h.ID
}

func hostRefusal(r *Run, q Request) string {
	if (q.Op == "attach" || q.Op == "review") && q.Host != nil && r != nil && (r.Status == "recovery-required" || r.Status == "unknown-outcome" || r.Recovery != nil) {
		return "run cannot progress; use recover and resolve its recovery options before starting or attaching to another run"
	}
	if q.Host != nil && !validHost(q.Host) {
		return "invalid host identity: name and id are required text"
	}
	if r != nil && r.Host != nil && !sameHost(r.Host, q.Host) {
		if q.Host == nil || q.Op != "attach" && q.Op != "new" || !hostReplaceable(r) {
			return "run is owned by host " + HostLabel(r.Host) + "; this client may only attach and inspect"
		}
	}
	if q.Host != nil && r != nil && r.Host == nil && q.Op != "new" {
		return "a standalone run owns this worktree; the host may replace it only with new"
	}
	if q.Host != nil && hostOwned[q.Op] != "" {
		return "host mode: the host owns " + hostOwned[q.Op]
	}
	if q.Host != nil && q.Op == "rerun" && (r == nil || r.Pause == nil) {
		return "host mode: the host owns cap decisions; only a paused review can be rerun"
	}
	return ""
}

func hostReplaceable(r *Run) bool {
	switch r.Status {
	case "running", "cancelling", "writing", "recovery-required", "unknown-outcome":
		return false
	}
	if r.Pause != nil && r.Pause.AutoResumeAt != "" {
		return false
	}
	if r.Recovery != nil || repairHeld(r.Repair) || pipelineCustody(r) {
		return false
	}
	if r.Review != nil && (r.Review.Status == "running" || r.Review.Status == "unknown-outcome") {
		return false
	}
	if r.Repair != nil && r.Repair.Writer == "unknown" {
		return false
	}
	if p := r.Pipeline; p != nil {
		if p.Active != nil {
			return false
		}
		for _, x := range p.Repairs {
			if x.Writer == "unknown" {
				return false
			}
		}
	}
	return true
}

func acceptHostHead(r *Run, reason string) error {
	if r.Host == nil || !ValidText(reason) {
		return errors.New("host head acceptance requires a host and reason")
	}
	if r.Review == nil || r.Review.Result == nil || r.Review.Rules.Candidate != r.Candidate || r.Review.Status != "clean-review" && r.Review.Status != "outstanding-findings" {
		return errors.New("host can accept only the current settled review head")
	}
	if slices.ContainsFunc(r.HostAcceptances, func(acceptance HostAcceptance) bool { return acceptance.Candidate == r.Candidate }) {
		return errors.New("candidate head already accepted")
	}
	r.HostAcceptances = append(r.HostAcceptances, HostAcceptance{Candidate: r.Candidate, Review: r.Review.ID, Actor: r.Host.Name, Reason: reason})
	r.Events = append(r.Events, Event{Kind: "host-accept-head", Actor: r.Host.Name, Reason: r.Candidate + ": " + reason})
	r.Token = opaque()
	return nil
}

func admitHostReview(r *Run, grant string, responses []ResolutionResponse) error {
	if !ValidText(grant) || len(grant) > 256 {
		return errors.New("host review requires grant: the host's review authorization identity")
	}
	if slices.Contains(r.HostGrants, grant) {
		return errors.New("grant already used: " + grant)
	}

	var response *Repair
	if len(responses) > 0 {
		response = &Repair{ID: opaque(), Stage: "review", Selected: latestHostFindings(r)}
		data, err := json.Marshal(map[string]any{"responses": responses})
		if err != nil {
			return err
		}
		response.Responses, err = parseResponseItems(string(data), response, true)
		if err != nil {
			return err
		}
	}
	previous := r.HostResponses
	r.HostResponses = response
	if err := admitReviewAgain(r); err != nil {
		r.HostResponses = previous
		return err
	}
	if response != nil {
		for _, answer := range response.Responses {
			if i := slices.IndexFunc(r.HostHistory, func(prior PriorFinding) bool { return prior.ID == answer.Finding }); i >= 0 {
				r.HostHistory[i].State = answer.Response
			}
		}
	}
	r.Review.Grant = grant
	r.HostGrants = append(r.HostGrants, grant)
	r.Events = append(r.Events, Event{Kind: "host-review", Actor: r.Host.Name, Reason: "grant " + grant})
	return nil
}

func classifyHostFindings(r *Run) error {
	base, _, err := repository.ReviewDiffBase(r.Submitted.Worktree, r.Target, r.Candidate)
	if err != nil {
		return err
	}
	diff, err := repository.Read(r.Submitted.Worktree, reviewDiffArgs(base, r.Candidate, "--unified=0")...)
	if err != nil {
		return errors.New("cannot classify carried host findings against current diff")
	}
	ranges := changedRanges(string(diff))
	findings := append([]Finding(nil), r.HostFindings...)
	if len(r.HostCarried) > 0 {
		findings = make([]Finding, 0, len(r.HostCarried))
		for _, carried := range r.HostCarried {
			findings = append(findings, carried.Finding)
		}
	}
	kept := make([]Finding, 0, len(findings))
	carriedFindings := make([]CarriedFinding, 0, len(findings))
	for _, finding := range findings {
		inside := false
		for _, span := range ranges[finding.File] {
			if finding.Start >= span[0] && finding.End <= span[1] {
				inside = true
				break
			}
		}
		item := CarriedFinding{Finding: finding, Kept: inside}
		if inside {
			item.Reason = "finding lines remain inside a changed hunk of the current target diff"
			kept = append(kept, finding)
		} else {
			item.Reason = "finding file or lines are outside changed hunks of the current target diff"
		}
		carriedFindings = append(carriedFindings, item)
	}
	r.HostCarried = carriedFindings
	r.HostFindings = kept
	return nil
}

func latestHostFindings(r *Run) []Finding {
	if x := r.Review; x != nil && x.Result != nil && (x.Status == "clean-review" || x.Status == "outstanding-findings") {
		return append([]Finding(nil), x.Result.Findings...)
	}
	findings := append([]Finding(nil), r.HostFindings...)
	for _, carried := range r.HostCarried {
		if !carried.Kept || slices.ContainsFunc(findings, func(f Finding) bool {
			return f.ID == carried.Finding.ID || f.File == carried.Finding.File && f.Start <= carried.Finding.End && carried.Finding.Start <= f.End
		}) {
			continue
		}
		findings = append(findings, carried.Finding)
	}
	return findings
}

func hostSuccessor(r *Run, next *Run, host *Host) {
	next.Host = host
	if !sameHost(r.Host, host) {
		return
	}
	next.Predecessor, r.Successor = r.ID, next.ID
	next.ReviewUsed, next.RepairUsed = r.ReviewUsed, r.RepairUsed
	next.Inherited = &Inheritance{From: r.ID, Review: r.ReviewUsed, Repair: r.RepairUsed}
	next.HostFindings = append([]Finding(nil), latestHostFindings(r)...)
	next.HostAcceptances = append([]HostAcceptance(nil), r.HostAcceptances...)
	next.HostHistory = append([]PriorFinding(nil), r.HostHistory...)
	for _, finding := range next.HostFindings {
		if !slices.ContainsFunc(next.HostHistory, func(prior PriorFinding) bool { return prior.ID == finding.ID }) {
			next.HostHistory = append(next.HostHistory, PriorFinding{Finding: finding, State: "open"})
		}
	}
	next.HostCarried = make([]CarriedFinding, 0, len(next.HostFindings))
	next.HostGrants = append([]string(nil), r.HostGrants...)
}

func hostResult(r *Run, current bool) *HostResult {
	x := r.Review
	if x == nil {
		return nil
	}
	v := &HostResult{Host: *r.Host, Grant: x.Grant, Review: x.ID, AcceptedHeads: append([]HostAcceptance(nil), r.HostAcceptances...), Head: r.Candidate, Target: r.Target, ReviewBase: x.ReviewBase, ReviewBaseReason: x.ReviewBaseReason, Status: x.Status, Current: current && x.Status != "stale", Findings: []Finding{}, Rules: x.Rules.Digest, Config: x.ConfigDigest, Input: x.InputDigest, Backend: x.Backend, Model: x.Config.Model}
	v.Reason, v.Diagnostic, v.Usage = x.Reason, x.Diagnostic, x.Usage
	v.Generated, v.Uncovered = x.Generated, x.Uncovered
	v.Carried = append([]CarriedFinding(nil), r.HostCarried...)
	if v.Model == "" {
		v.Model = x.ReportedModel
		if v.Model == "" && x.Result != nil {
			v.Model = x.Result.ReportedModel
		}
	}
	if x.Result != nil {
		v.Verdict, v.Findings = x.Result.Verdict, append([]Finding(nil), x.Result.Findings...)
		v.Dismissed = append([]Finding(nil), x.Result.Dismissed...)
		v.Conflicts = append([]FindingConflict(nil), x.Result.Conflicts...)
		v.DismissalReasons = x.Result.DismissalReasons
		v.Resolutions = x.Result.Resolutions
	}
	settled := x.Result != nil && (x.Status == "clean-review" || x.Status == "outstanding-findings")
	for _, carried := range r.HostCarried {
		if settled || !carried.Kept || slices.ContainsFunc(v.Findings, func(f Finding) bool {
			return f.ID == carried.Finding.ID || f.File == carried.Finding.File && f.Start <= carried.Finding.End && carried.Finding.Start <= f.End
		}) {
			continue
		}
		finding := carried.Finding
		finding.Head = r.Candidate
		v.Findings = append(v.Findings, finding)
		if x.Status != "inconclusive" && x.Status != "cancelled" && x.Status != "stale" {
			v.Verdict = "findings"
			v.Status = "outstanding-findings"
		}
	}
	return v
}

func hostView(v *Response, r *Run, current, actionable bool) {
	actions := []string{}
	for _, a := range v.Actions {
		if slices.Contains(hostActions, a) {
			actions = append(actions, a)
		}
	}
	if actionable && r.Intent != "" && r.Target != "" && r.Reviewer != nil && v.RulesReady && !slices.Contains(actions, "review") {
		actions = append(actions, "review")
	}
	if actionable && r.Review != nil && r.Review.Result != nil && (r.Review.Status == "clean-review" || r.Review.Status == "outstanding-findings") && !slices.ContainsFunc(r.HostAcceptances, func(acceptance HostAcceptance) bool { return acceptance.Candidate == r.Candidate }) {
		actions = append(actions, "accept-head")
	}
	v.Actions = actions
	v.Blocked = append(v.Blocked, "owned by host "+HostLabel(r.Host)+": the host owns review outcomes, author repair, candidate custody, publication, CI and cap decisions")
	v.HostResult = hostResult(r, current)
}

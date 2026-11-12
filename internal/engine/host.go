package engine

import (
	"errors"
	"slices"
)

type Host struct {
	Name     string            `json:"name"`
	ID       string            `json:"id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type HostResult struct {
	Host     Host      `json:"host"`
	Grant    string    `json:"grant"`
	Review   string    `json:"review_id"`
	Head     string    `json:"head"`
	Target   string    `json:"target"`
	Status   string    `json:"status"`
	Verdict  string    `json:"verdict,omitempty"`
	Current  bool      `json:"current"`
	Findings []Finding `json:"findings"`
	Rules    string    `json:"rules_digest"`
	Config   string    `json:"config_digest"`
	Input    string    `json:"input_digest"`
	Backend  string    `json:"backend"`
	Model    string    `json:"model"`

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

var hostActions = []string{"attach", "inspect", "apply", "resolve-rules", "prepare", "review", "cancel", "resume", "rerun", "recover", "new"}

func validHost(h *Host) bool {
	if h == nil || !validText(h.Name) || !validText(h.ID) || len(h.Name) > 128 || len(h.ID) > 256 || len(h.Metadata) > 16 {
		return false
	}
	for k, v := range h.Metadata {
		if !validText(k) || len(k) > 64 || len(v) > 256 || v != "" && !validText(v) {
			return false
		}
	}
	return true
}

func sameHost(a, b *Host) bool {
	return a != nil && b != nil && a.Name == b.Name && a.ID == b.ID
}

func hostLabel(h *Host) string {
	return h.Name + " " + h.ID
}

func hostRefusal(r *Run, q Request) string {
	if q.Host != nil && !validHost(q.Host) {
		return "invalid host identity: name and id are required text"
	}
	if r != nil && r.Host != nil && !sameHost(r.Host, q.Host) {
		return "run is owned by host " + hostLabel(r.Host) + "; this client may only attach and inspect"
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

func admitHostReview(r *Run, grant string) error {
	if !validText(grant) || len(grant) > 256 {
		return errors.New("host review requires grant: the host's review authorization identity")
	}
	if slices.Contains(r.HostGrants, grant) {
		return errors.New("grant already used: " + grant)
	}
	if err := admitReviewAgain(r); err != nil {
		return err
	}
	r.Review.Grant = grant
	r.HostGrants = append(r.HostGrants, grant)
	r.Events = append(r.Events, Event{Kind: "host-review", Actor: r.Host.Name, Reason: "grant " + grant})
	return nil
}

func hostSuccessor(r *Run, next *Run, host *Host) {
	next.Host = host
	if !sameHost(r.Host, host) {
		return
	}
	next.Predecessor, r.Successor = r.ID, next.ID
	next.ReviewUsed, next.RepairUsed = r.ReviewUsed, r.RepairUsed
	next.Inherited = &Inheritance{From: r.ID, Review: r.ReviewUsed, Repair: r.RepairUsed}
	next.HostGrants = append([]string(nil), r.HostGrants...)
}

func hostResult(r *Run, current bool) *HostResult {
	x := r.Review
	if x == nil {
		return nil
	}
	v := &HostResult{Host: *r.Host, Grant: x.Grant, Review: x.ID, Head: r.Candidate, Target: r.Target, ReviewBase: x.ReviewBase, ReviewBaseReason: x.ReviewBaseReason, Status: x.Status, Current: current && x.Status != "stale", Findings: []Finding{}, Rules: x.Rules.Digest, Config: x.ConfigDigest, Input: x.InputDigest, Backend: x.Backend, Model: x.Config.Model}
	v.Reason, v.Diagnostic, v.Usage = x.Reason, x.Diagnostic, x.Usage
	if v.Model == "" {
		v.Model = x.ReportedModel
		if v.Model == "" && x.Result != nil {
			v.Model = x.Result.ReportedModel
		}
	}
	if x.Result != nil {
		v.Verdict, v.Findings = x.Result.Verdict, x.Result.Findings
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
	v.Actions = actions
	v.Blocked = append(v.Blocked, "owned by host "+hostLabel(r.Host)+": the host owns author repair, candidate custody, publication, CI and cap decisions")
	v.HostResult = hostResult(r, current)
}

package engine

import (
	"github.com/Thanhbinh1905/greenhouse/internal/forge"
	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

type DeliveryView struct {
	PushedHead string             `json:"pushed_head"`
	PRNumber   int                `json:"pr_number"`
	PRURL      string             `json:"pr_url"`
	Checks     []forge.CheckState `json:"checks"`
	Current    bool               `json:"current"`
}

func deliveryView(r *Run, source repository.Identity, sourceErr error) *DeliveryView {
	if r.Pipeline == nil {
		return nil
	}
	p := r.Pipeline
	v := &DeliveryView{Checks: []forge.CheckState{}}
	push, pr, ci := p.stage("push"), p.stage("pr"), p.stage("ci")
	if push != nil && push.Status == "passed" {
		v.PushedHead = push.Head
	}
	if pr != nil {
		v.PRNumber, v.PRURL = pr.Number, pr.URL
	}
	if ci != nil && len(ci.Checks) > 0 {
		v.Checks = ci.Checks
	} else {
		for _, name := range p.Plan.RequiredChecks {
			v.Checks = append(v.Checks, forge.CheckState{Name: name, Status: "unobserved", Head: r.Candidate})
		}
	}
	if sourceErr != nil || source != r.Submitted || v.PushedHead != r.Candidate || pr == nil || pr.Head != r.Candidate || ci == nil || ci.Head != r.Candidate || ci.Status != "passed" {
		return v
	}
	if len(p.Plan.RequiredChecks) == 0 || len(ci.Checks) != len(p.Plan.RequiredChecks) {
		return v
	}
	remaining := make(map[string]bool, len(p.Plan.RequiredChecks))
	for _, name := range p.Plan.RequiredChecks {
		remaining[name] = true
	}
	for _, check := range ci.Checks {
		if !remaining[check.Name] || check.Status != "completed" || check.Conclusion == nil || *check.Conclusion != "success" || check.Head != r.Candidate {
			return v
		}
		delete(remaining, check.Name)
	}
	if len(remaining) != 0 {
		return v
	}
	candidate, candidateErr := repository.Resolve(p.Worktree)
	v.Current = candidateErr == nil && candidate.Head == r.Candidate && candidate.Branch == p.Ref
	return v
}

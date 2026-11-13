package main

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// PlanGrant is an expanded fixer authority declared in the trusted per-user pipeline.json.
type PlanGrant struct {
	Authority string   `json:"authority"`
	Stages    []string `json:"stages"`
	Paths     []string `json:"paths"`
	Reason    string   `json:"reason"`
}

type Grant struct {
	ID        string   `json:"id"`
	Authority string   `json:"authority"`
	Stages    []string `json:"stages"`
	Paths     []string `json:"paths"`
	Source    string   `json:"source"`
	Actor     string   `json:"actor,omitempty"`
	Reason    string   `json:"reason"`
	Decision  string   `json:"decision,omitempty"`
}

var grantStages = map[string]bool{"review": true, "test": true, "lint": true, "docs": true, "ci": true}

// grantAuthority maps a parked authority category to the authority a grant can give.
var grantAuthority = map[string]string{"add-files": "add-files", "destructive": "delete-files"}

func validGrantPath(name string) bool {
	dir := strings.HasSuffix(name, "/")
	clean := strings.TrimSuffix(name, "/")
	return clean != "" && !filepath.IsAbs(clean) && filepath.Clean(clean) == clean && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && (!dir || !strings.HasSuffix(clean, "/"))
}

func validatePlanGrants(grants []PlanGrant) error {
	if len(grants) > 16 {
		return errors.New("at most 16 grants")
	}
	for _, g := range grants {
		if g.Authority != "add-files" && g.Authority != "delete-files" {
			return errors.New("grant authority must be add-files or delete-files")
		}
		if len(g.Stages) == 0 || len(g.Paths) == 0 || len(g.Paths) > 64 || !validText(g.Reason) {
			return errors.New("each grant needs stages, paths and a reason")
		}
		for _, stage := range g.Stages {
			if !grantStages[stage] {
				return errors.New("grant stages are review, test, lint, docs and ci")
			}
		}
		for _, name := range g.Paths {
			if !validGrantPath(name) {
				return errors.New("grant paths must be clean repository-relative files, or directories ending in /")
			}
		}
	}
	return nil
}

func planGrants(plan CommandPlan) []Grant {
	var out []Grant
	for _, g := range plan.Grants {
		out = append(out, Grant{ID: opaque(), Authority: g.Authority, Stages: g.Stages, Paths: g.Paths, Source: "pipeline.json", Reason: g.Reason})
	}
	return out
}

func stageGrants(p *Pipeline, stage string) []Grant {
	var out []Grant
	for _, g := range p.Grants {
		if has(g.Stages, stage) {
			out = append(out, g)
		}
	}
	return out
}

func covered(grants []Grant, authority, name string) bool {
	for _, g := range grants {
		if g.Authority != authority {
			continue
		}
		for _, scope := range g.Paths {
			if name == scope || strings.HasSuffix(scope, "/") && strings.HasPrefix(name, scope) {
				return true
			}
		}
	}
	return false
}

// grantedPaths returns the paths a repair changed under a grant.
func grantedPaths(x *Repair) []string {
	var out []string
	for _, entry := range x.Granted {
		parts := strings.SplitN(entry, ":", 2)
		name := ""
		if len(parts) == 2 {
			name = parts[1]
		}
		out = append(out, name)
	}
	return out
}

func classifyGranted(x *Repair, added, deleted []string) error {
	x.Granted = nil
	var addMissing, deleteMissing []string
	for _, name := range added {
		if covered(x.Grants, "add-files", name) {
			x.Granted = append(x.Granted, "add-files:"+name)
		} else {
			addMissing = append(addMissing, name)
		}
	}
	for _, name := range deleted {
		if covered(x.Grants, "delete-files", name) {
			x.Granted = append(x.Granted, "delete-files:"+name)
		} else {
			deleteMissing = append(deleteMissing, name)
		}
	}
	sort.Strings(x.Granted)
	if len(deleteMissing) > 0 {
		return authorityError{"destructive", "the fixer deleted " + strings.Join(deleteMissing, ", "), deleteMissing}
	}
	if len(addMissing) > 0 {
		return authorityError{"add-files", "the fixer added " + strings.Join(addMissing, ", "), addMissing}
	}
	return nil
}

// refuseAuthority keeps the refused edits as partial evidence and restores the candidate for a later retry.
func refuseAuthority(x *Repair) {
	capturePartial(x)
	x.Status = "needs-authority"
	if err := restoreCandidate(x); err != nil {
		x.Status, x.Writer = "unknown-outcome", "unknown"
		x.Log += "; candidate restore failed: " + err.Error()
	}
}

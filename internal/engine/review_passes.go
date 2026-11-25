package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

func mergeReviewPasses(passes []reviewOutcome, paths []string) reviewOutcome {
	merged := reviewOutcome{Status: "inconclusive", Log: "merged independent review passes"}
	if len(passes) == 0 {
		return merged
	}
	coverage := map[string]bool{}
	findings := []Finding{}
	var uncovered []UncoveredPath
	for _, pass := range passes {
		merged.Answer.Usage = addUsage(merged.Answer.Usage, pass.Answer.Usage)
		merged.Log += fmt.Sprintf("; pass %s: %s", pass.Status, pass.Log)
		if pass.Status == "cancelled" || pass.Status == "paused" || pass.Status == "unconfigured" || pass.Status == "stale" {
			if merged.Status == "inconclusive" {
				merged.Status, merged.Failure = pass.Status, pass.Failure
			}
		}
		if pass.Confinement != nil && merged.Confinement == nil {
			merged.Confinement = pass.Confinement
		}
		if merged.Snapshot == "" || strings.HasPrefix(pass.Snapshot, "modified") {
			merged.Snapshot = pass.Snapshot
		}
		if pass.Result == nil {
			continue
		}
		if merged.Result == nil {
			merged.Result = &Verdict{Head: pass.Result.Head, Findings: []Finding{}, ReportedProvider: pass.Result.ReportedProvider, ReportedModel: pass.Result.ReportedModel}
		}
		mergeVerdictRecords(merged.Result, pass.Result)
		for _, path := range pass.Result.Coverage {
			coverage[path] = true
		}
		uncovered = append(uncovered, pass.Result.Uncovered...)
		for _, finding := range pass.Result.Findings {
			duplicate := false
			for i := range findings {
				prior := &findings[i]
				if prior.File == finding.File && prior.Start <= finding.End && finding.Start <= prior.End {
					start, end := min(prior.Start, finding.Start), max(prior.End, finding.End)
					if priorityRank(finding.Priority) < priorityRank(prior.Priority) {
						*prior = finding
					}
					prior.Start, prior.End = start, end
					duplicate = true
					break
				}
			}
			if !duplicate {
				findings = append(findings, finding)
			}
		}
	}
	if strings.HasPrefix(merged.Snapshot, "modified") {
		merged.Status, merged.Result = "inconclusive", nil
	}
	if merged.Result == nil {
		merged.Answer.Tokens, merged.Answer.CostUSD = merged.Answer.Usage.Tokens, merged.Answer.Usage.CostUSD
		return merged
	}
	for path := range coverage {
		merged.Result.Coverage = append(merged.Result.Coverage, path)
	}
	slices.Sort(merged.Result.Coverage)
	for _, path := range paths {
		if coverage[path] {
			continue
		}
		reason := "not covered by reviewer within budget"
		for _, item := range uncovered {
			if item.Path == path {
				reason = item.Reason
				break
			}
		}
		merged.Result.Uncovered = append(merged.Result.Uncovered, UncoveredPath{Path: path, Reason: reason})
	}
	slices.SortFunc(merged.Result.Uncovered, func(a, b UncoveredPath) int { return strings.Compare(a.Path, b.Path) })
	merged.Result.Findings = findings
	switch {
	case len(findings) > 0:
		merged.Status, merged.Result.Verdict = "outstanding-findings", "findings"
	case len(merged.Result.Uncovered) > 0:
		merged.Status, merged.Result.Verdict = "partial-review", "partial"
	default:
		merged.Status, merged.Result.Verdict = "clean-review", "clean"
	}
	merged.Answer.Tokens, merged.Answer.CostUSD = merged.Answer.Usage.Tokens, merged.Answer.Usage.CostUSD
	return merged
}

func mergeVerdictRecords(into, from *Verdict) {
	into.Diagnostics = append(into.Diagnostics, from.Diagnostics...)
	into.RejectedFindings = append(into.RejectedFindings, from.RejectedFindings...)
	into.Dismissed = append(into.Dismissed, from.Dismissed...)
	into.Conflicts = append(into.Conflicts, from.Conflicts...)
	for id, reason := range from.DismissalReasons {
		if into.DismissalReasons == nil {
			into.DismissalReasons = map[string]string{}
		}
		if _, ok := into.DismissalReasons[id]; !ok {
			into.DismissalReasons[id] = reason
		}
	}
	if from.Documentation != nil && (into.Documentation == nil || from.Documentation.Status == "missing" && into.Documentation.Status != "missing") {
		doc := *from.Documentation
		into.Documentation = &doc
	}
	for _, check := range from.Resolutions {
		i := slices.IndexFunc(into.Resolutions, func(c ResolutionCheck) bool { return c.Finding == check.Finding })
		switch {
		case i < 0:
			into.Resolutions = append(into.Resolutions, check)
		case check.Check == "rejected" && into.Resolutions[i].Check != "rejected":
			into.Resolutions[i] = check
		}
	}
}

func priorityRank(priority string) int {
	switch priority {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	default:
		return 3
	}
}

func confirmMergedReview(ctx context.Context, x *Execution, done reviewOutcome) reviewOutcome {
	if done.Status != "outstanding-findings" || done.Result == nil || len(blockingFindings(done.Result.Findings)) == 0 {
		return done
	}
	dir := reviewDir(x.ID + "-merged")
	if err := os.Mkdir(dir, 0700); err != nil {
		return done
	}
	defer func() { _ = os.RemoveAll(dir) }()
	tree := filepath.Join(dir, "tree")
	if x.Snapshot == nil || repository.Materialize(x.Source.Worktree, x.Snapshot, tree) != nil {
		return done
	}
	defer repository.RemoveSnapshot(tree)
	return confirmBlockingFindings(ctx, x, dir, tree, done)
}

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

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
		if pass.Result == nil {
			continue
		}
		if merged.Result == nil {
			merged.Result = &Verdict{Head: pass.Result.Head, Findings: []Finding{}}
		}
		merged.Result.Diagnostics = append(merged.Result.Diagnostics, pass.Result.Diagnostics...)
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

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/runner"
)

type Confirmation struct {
	Status   string `json:"status"`
	Reported string `json:"reported_priority"`
	Evidence string `json:"evidence"`
}

const confirmationContract = ` This is a confirmation pass for one reported finding. Verify only that finding. Coverage of other changed paths is not required. Return a finding that overlaps the reported lines only if the defect is real in the committed code, with its priority and the code evidence. Return no finding for it if the defect is not real.`

func confirmationInput(x *Execution, f Finding) ([]byte, error) {
	header, evidence, ok := strings.Cut(string(x.Input), "\n\nDiff:\n")
	if !ok {
		return nil, errors.New("review input has no diff section")
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(header), &metadata); err != nil {
		return nil, fmt.Errorf("decode review input: %w", err)
	}
	contract := reviewContract + confirmationContract
	if len(x.Generated) > 0 {
		contract += generatedContract
	}
	if len(x.Uncovered) > 0 {
		contract += uncoveredContract
	}
	intent := fmt.Sprintf("Independently verify this reported defect against the changed code. Inspect %s lines %d-%d. Report a blocking finding only if the defect is real; explain the code evidence and behavior. Otherwise return clean.\n\nReported finding: %s\nProposed action: %s", f.File, f.Start, f.End, f.Explanation, f.Action)
	for key, value := range map[string]any{"Intent": intent, "Contract": contract} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		metadata[key] = encoded
	}
	delete(metadata, "Responses")
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return append(append(encoded, "\n\nDiff:\n"...), evidence...), nil
}

func confirmationArgs(x *Execution) ([]string, error) {
	schema := verdictSchema(false, false)
	if !x.Config.Native {
		return legacyAgentArgs(x.Config, "reviewer", schema, reviewerPrompt), nil
	}
	return agentArgs(x.Config, "reviewer", schema, reviewerPrompt)
}

func confirmBlockingFindings(ctx context.Context, x *Execution, dir, tree string, done reviewOutcome) reviewOutcome {
	if done.Status != "outstanding-findings" || done.Result == nil || len(blockingFindings(done.Result.Findings)) == 0 {
		return done
	}
	args, err := confirmationArgs(x)
	if err != nil {
		return done
	}
	result := *done.Result
	result.Findings = []Finding{}
	for i, f := range done.Result.Findings {
		if f.Priority != "P0" && f.Priority != "P1" {
			result.Findings = append(result.Findings, f)
			continue
		}
		pass := *x
		pass.ID, pass.Full, pass.Responses, pass.Args, pass.Uncovered = fmt.Sprintf("%s-confirm-%d", x.ID, i+1), false, nil, args, slices.Clone(x.Uncovered)
		confirmation, priority, outcome := confirmFinding(ctx, &pass, filepath.Join(dir, fmt.Sprintf("confirm-%d", i+1)), tree, f)
		done.InputDigests = mergeInputDigests(done.InputDigests, outcome.InputDigests)
		done.Answer.Usage = addUsage(done.Answer.Usage, outcome.Answer.Usage)
		done.Answer.Tokens, done.Answer.CostUSD = done.Answer.Usage.Tokens, done.Answer.Usage.CostUSD
		if outcome.Status == "cancelled" {
			done.Status, done.Result, done.Log = "cancelled", nil, done.Log+"; "+outcome.Log
			done.Answer.Reason, done.Answer.Diagnostic = outcome.Answer.Reason, outcome.Answer.Diagnostic
			return done
		}
		done.Log += fmt.Sprintf("; finding %s %s", f.ID, confirmation.Status)
		f.Confirmation = &confirmation
		if confirmation.Status == "dropped" {
			result.Dismissed = append(result.Dismissed, f)
			continue
		}
		f.Priority = priority
		result.Findings = append(result.Findings, f)
	}
	if len(result.Findings) == 0 {
		result.Verdict, done.Status = "clean", "clean-review"
		if len(result.Uncovered) > 0 {
			result.Verdict, done.Status = "partial", "partial-review"
		}
	}
	done.Result = &result
	return done
}

func confirmFinding(ctx context.Context, x *Execution, dir, tree string, f Finding) (Confirmation, string, reviewOutcome) {
	c := Confirmation{Reported: f.Priority}
	input, err := confirmationInput(x, f)
	if err == nil {
		err = os.Mkdir(dir, 0700)
	}
	if err != nil {
		c.Status, c.Evidence = "unconfirmed", "confirmation pass could not start: "+err.Error()
		return c, f.Priority, reviewOutcome{}
	}
	x.Input, x.InputDigest = input, runner.BytesDigest(input)
	outcome, _ := runBoundedReview(ctx, x, dir, tree)
	settled := outcome.Result != nil && (outcome.Status == "clean-review" || outcome.Status == "outstanding-findings" || outcome.Status == "partial-review")
	if !settled {
		c.Status, c.Evidence = "unconfirmed", "confirmation pass did not settle: "+outcome.Status
		return c, f.Priority, outcome
	}
	var match *Finding
	for i, g := range outcome.Result.Findings {
		if g.File == f.File && g.Start <= f.End && f.Start <= g.End && (match == nil || g.Priority < match.Priority) {
			match = &outcome.Result.Findings[i]
		}
	}
	if match != nil && (match.Priority == "P0" || match.Priority == "P1") {
		c.Status, c.Evidence = "confirmed", match.Explanation
		return c, f.Priority, outcome
	}
	covered := slices.Contains(outcome.Result.Coverage, f.File) && !slices.ContainsFunc(outcome.Result.Uncovered, func(u UncoveredPath) bool { return u.Path == f.File })
	if !covered {
		c.Status, c.Evidence = "unconfirmed", fmt.Sprintf("confirmation pass did not cover %s", f.File)
		return c, f.Priority, outcome
	}
	if match == nil {
		c.Status, c.Evidence = "dropped", fmt.Sprintf("confirmation pass found no defect at %s:%d-%d", f.File, f.Start, f.End)
		return c, f.Priority, outcome
	}
	c.Status, c.Evidence = "demoted", match.Explanation
	return c, match.Priority, outcome
}

func markUnconfirmed(done *reviewOutcome) {
	if done.Status != "outstanding-findings" || done.Result == nil {
		return
	}
	for i, f := range done.Result.Findings {
		if (f.Priority == "P0" || f.Priority == "P1") && f.Confirmation == nil {
			done.Result.Findings[i].Confirmation = &Confirmation{Status: "unconfirmed", Reported: f.Priority, Evidence: "the controller restarted before a confirmation pass completed"}
		}
	}
}

func dismissFindings(r *Run, dismissed []Finding) {
	for _, d := range dismissed {
		r.Findings = slices.DeleteFunc(r.Findings, func(f Finding) bool { return f.ID == d.ID })
	}
}

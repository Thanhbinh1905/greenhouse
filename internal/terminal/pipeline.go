package terminal

import (
	"fmt"
	"io"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

var stageMarkers = map[string]string{
	"passed":              "[pass]",
	"repaired-unreviewed": "[REPAIRED, NOT REVIEWED]",
	"failed":              "[FAIL]",
	"not-applicable":      "[n/a]",
	"unconfigured":        "[UNCONFIGURED]",
	"needs-decision":      "[DECISION]",
	"accepted-risk":       "[ACCEPTED RISK]",
	"unknown-outcome":     "[UNKNOWN]",
	"running":             "[running]",
	"pending":             "[ ]",
	"stale":               "[stale]",
	"cancelled":           "[cancelled]",
	"unsupported":         "[unsupported]",
	"paused":              "[PAUSED]",
}

func renderPipeline(w io.Writer, r *engine.Run, details bool) {
	p := r.Pipeline
	fmt.Fprintf(w, "Pipeline: %s", p.Status)
	if p.Failed != "" && p.Status != "running" {
		fmt.Fprintf(w, " | stopped at %s", p.Failed)
	}
	fmt.Fprintf(w, "\nGate repairs: %d/%d (counted apart from the review cap)\n", p.RepairUsed, p.RepairLimit)
	if outcome := engine.OutcomeLabel(p.Outcome); outcome != "" && (p.Status == "locally-ready" || p.Status == "ci-ready") {
		fmt.Fprintln(w, "Outcome: "+outcome)
	}
	if g := p.FinalFix; g != nil {
		fmt.Fprintf(w, "Final-fix by %s: repair %d/1, final review %d/1\n", g.Actor, g.Repair, g.Review)
	}
	for _, d := range p.Decisions {
		if d.Answer != nil {
			fmt.Fprintf(w, "Decided %s: %s by %s: %s\n", d.Kind, d.Answer.Choice, d.Answer.Actor, d.Answer.Reason)
		}
	}
	if d := p.Open(); d != nil {
		fmt.Fprintf(w, "DECISION REQUIRED: %s | gate %s\n", d.Kind, engine.Short(d.ID))
		if d.UserOnly {
			fmt.Fprintf(w, "User-only authority: %s\n", d.Category)
		}
		fmt.Fprintln(w, d.Reason)
		if len(d.Findings) > 0 {
			var ids []string
			width := terminalWidth()
			for _, id := range d.Findings {
				ids = append(ids, displayID(id, width))
			}
			fmt.Fprintln(w, "Grouped findings: "+strings.Join(ids, ", "))
		}
		fmt.Fprintf(w, "Options: %s\n[d] answers this decision\n", strings.Join(d.Options, ", "))
	}
	if p.Worktree != "" {
		fmt.Fprintf(w, "Candidate ref: %s\n", strings.TrimPrefix(p.Ref, "refs/heads/"))
		if details {
			fmt.Fprintf(w, "Candidate worktree: %s\n", p.Worktree)
		}
	}
	if n := len(p.Repairs); n > 0 && p.Repairs[n-1].Confinement != nil {
		fmt.Fprintln(w, confinementLine(p.Repairs[n-1].Confinement))
	}
	if p.Handback != "" {
		fmt.Fprintf(w, "Handback: %s %s\n", p.Handback, p.HandbackError)
	}
	if len(p.Conflict) > 0 {
		fmt.Fprintf(w, "Target conflict: %s\n[m] authorizes the fixer\n", strings.Join(p.Conflict, ", "))
	}
	for _, st := range p.Stages {
		marker := stageMarkers[st.Status]
		if marker == "" {
			marker = "[" + st.Status + "]"
		}
		if st.Status == "passed" && st.Repairs > 0 {
			marker = "[pass after repair, rereviewed]"
		}
		fmt.Fprintf(w, "%s %s\n", marker, st.Name)
		if st.Head != "" {
			fmt.Fprintf(w, "  head %s\n", st.Head)
		}
		if st.URL != "" {
			fmt.Fprintf(w, "  PR %s\n", st.URL)
		}
		if st.Reason != "" {
			fmt.Fprintln(w, "  "+st.Reason)
		}
		if st.Inputs != "" {
			fmt.Fprintf(w, "  in %s", st.Inputs[:8])
			if st.Candidate != "" {
				fmt.Fprintf(w, " cand %s", st.Candidate[:8])
			}
			fmt.Fprintf(w, " runs %d fixes %d\n", st.Runs, st.Repairs)
		}
		for _, c := range st.Commands {
			args := c.Args
			if !details {
				args = engine.DisplayArgs(args)
			}
			fmt.Fprintf(w, "  $ %s\n    exit=%d %s\n", strings.Join(args, " "), c.Exit, c.Status)
			if c.Output == "" || c.Status == "passed" && !details {
				continue
			}
			lines := strings.Split(strings.TrimRight(c.Output, "\n"), "\n")
			if !details && len(lines) > 3 {
				lines = lines[len(lines)-3:]
			}
			for _, line := range lines {
				fmt.Fprintln(w, "    | "+line)
			}
		}
	}
}

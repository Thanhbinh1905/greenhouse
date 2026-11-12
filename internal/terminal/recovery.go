package terminal

import (
	"fmt"
	"io"
	"strings"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
)

func renderRecovery(w io.Writer, r *engine.Run) {
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
		fmt.Fprintf(w, "Executor %s: %s exit=%d %s\n", engine.Short(e.ID[:strings.LastIndex(e.ID, ".")])+e.ID[strings.LastIndex(e.ID, "."):], e.State, e.Exit, e.Detail)
	}
	if c := rec.Candidate; c != nil {
		observed := engine.Short(c.Observed)
		if observed == "" {
			observed = "none"
		}
		fmt.Fprintf(w, "Candidate %s: %s\n  recorded %s observed %s\n", strings.TrimPrefix(c.Ref, "refs/heads/"), c.Summary, engine.Short(c.Durable), observed)
	}
	if rec.Adopt != "" {
		fmt.Fprintln(w, "Adopt: "+rec.Adopt)
	}
	fmt.Fprintf(w, "Options: %s\n[e] answers recovery gate %s\n", strings.Join(rec.Options, ", "), engine.Short(rec.ID))
}

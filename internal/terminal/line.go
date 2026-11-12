package terminal

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unicode"
	"unsafe"

	"github.com/Thanhbinh1905/greenhouse/internal/engine"
	"github.com/Thanhbinh1905/greenhouse/internal/repository"
)

func call(h string, q engine.Request) (engine.Response, error) {
	return engine.Send(h, q, true)
}

func terminalWidth() int {
	size := [4]uint16{}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size))); err == 0 && size[1] > 1 {
		return int(size[1]) - 1
	}
	return 80
}
func wrapWords(text string, width int) string {
	indent := text[:len(text)-len(strings.TrimLeft(text, " "))]
	if len(indent) >= width-1 {
		indent = ""
	}
	available := width - len(indent)
	var lines []string
	line := indent
	count := 0
	rest := strings.TrimLeft(text, " ")
	for _, word := range strings.Fields(text) {
		at := strings.Index(rest, word)
		gap := rest[:at]
		rest = rest[at+len(word):]
		n := len([]rune(word))
		if n > available {
			if count > 0 {
				lines = append(lines, line)
			}
			lines, line, count = append(lines, word), indent, 0
			continue
		}
		if count > 0 && count+len([]rune(gap))+n > available {
			lines, line, count = append(lines, line), indent, 0
		}
		if count > 0 {
			line, count = line+gap, count+len([]rune(gap))
		}
		line, count = line+word, count+n
	}
	if count > 0 || len(lines) == 0 {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
func wrapMenuEntries(entries []string, width int) string {
	var lines []string
	line := ""
	for _, entry := range entries {
		if line != "" && len([]rune(line))+2+len([]rune(entry)) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		line += entry
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
func displayID(id string, width int) string {
	runes := []rune(id)
	available := width - 1
	if len(runes) <= available {
		return id
	}
	left := (available - 1) / 2
	return string(runes[:left]) + "…" + string(runes[len(runes)-(available-1-left):])
}
func terminalText(text string) string {
	width := terminalWidth()
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		clean := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, line)
		lines[i] = wrapWords(clean, width)
	}
	return strings.Join(lines, "\n")
}
func statePage(v engine.Response, live string, details bool) string {
	var page strings.Builder
	r := v.Run
	s := r.Configs[len(r.Configs)-1]
	if s.Settings.Preset == "full" {
		fmt.Fprintln(&page, "Greenhouse | full pipeline")
	} else {
		fmt.Fprintln(&page, "Greenhouse | independent review")
	}
	fmt.Fprintf(&page, "Events: %s\n", live)
	fmt.Fprintf(&page, "Run %s\nStage: %s | %s\nCandidate: %s\nMode: %s | %s | limit %d\nReviewer: %s / %s\nFixer: %s / %s\nBudget: review %d + repair %d = %d of %d\nRules: %s\nSources: %s\n", r.ID, r.Stage, r.Status, r.Candidate, s.Settings.Mode, s.Settings.Preset, s.Settings.Limit, s.Settings.ReviewerBackend, s.Settings.ReviewerModel, s.Settings.FixerBackend, s.Settings.FixerModel, r.ReviewUsed, r.RepairUsed, r.ReviewUsed+r.RepairUsed, s.Settings.Limit, strings.Join(s.Settings.Rules, ", "), engine.SourceSummary(engine.FieldSources(s)))
	if r.Target != "" {
		fmt.Fprintf(&page, "Target: %s\nIntent: %s\nScope: committed target-to-candidate diff; dirty files excluded\n", r.Target, r.Intent)
	}
	if o := v.Repository; o != nil {
		fmt.Fprintln(&page, engine.OverrideLine(o))
	}
	for _, cfg := range v.Config {
		if cfg.Status == "retired" {
			fmt.Fprintf(&page, "Configuration: %s retired; preserved, not used for new reviews\n", cfg.File)
		}
	}
	if x := v.HostResult; x != nil {
		outcome := x.Verdict
		if outcome == "" {
			outcome = x.Status
		}
		fmt.Fprintln(&page, wrapWords(fmt.Sprintf("Host: %s | grant %s | %s | current %t", engine.HostLabel(&x.Host), x.Grant, outcome, x.Current), terminalWidth()))
	}
	for _, rule := range v.Rules {
		fmt.Fprintf(&page, "Rule %s | %s | required=%t\nSource: %s\nDigest: %s\n", rule.Selection, rule.Status, rule.Required, rule.Source, rule.Digest)
		if rule.Error != "" {
			fmt.Fprintln(&page, "Load error: "+rule.Error)
		}
		if details && rule.Pack != nil {
			fmt.Fprintln(&page, v.RuleScope)
			for _, c := range rule.Pack.Criteria {
				fmt.Fprintf(&page, "%s/%s [%s]: %s\nRepair: %s\n", rule.Pack.ID, c.ID, c.Classification, c.Text, c.Repair)
			}
		}
	}
	fmt.Fprintf(&page, "Auto-resume: %s\n", engine.AutoResumeLabel(s.Settings))
	if p := r.Pause; p != nil {
		width := terminalWidth()
		fmt.Fprintln(&page, wrapWords("PAUSED: "+v.Explanation, width))
		fmt.Fprintln(&page, wrapWords(fmt.Sprintf("Paused stage: %s | agent %s %s | backend %s", p.Stage, p.Backend, p.Model, p.Reason), width))
		if p.ResetAt != "" {
			fmt.Fprintf(&page, "Reset: %s\n", engine.Clock(p.ResetAt))
		}
		if p.AutoResumeAt != "" {
			fmt.Fprintf(&page, "Scheduled %s: %s\n", p.AutoAction, engine.Clock(p.AutoResumeAt))
		}
	}
	for _, b := range v.Blocked {
		fmt.Fprintln(&page, "Blocked: "+b)
	}
	renderRecovery(&page, r)
	if r.Pipeline != nil {
		renderPipeline(&page, r, details)
	}
	if details && r.SelectedDiff != "" {
		fmt.Fprintln(&page, "Selected committed diff:", r.SelectedDiff)
	}
	actionable, informational := engine.Outstanding(r)
	label := map[string]string{}
	for _, f := range actionable {
		label[f.ID] = "Outstanding"
	}
	for _, f := range informational {
		label[f.ID] = "Informational"
	}
	kinds := engine.ResolvedKinds(r)
	responses := engine.ResponseLines(r)
	for _, f := range r.Findings {
		state := label[f.ID]
		if state == "" {
			state = "Resolved (" + kinds[f.ID] + ")"
		}
		fmt.Fprintf(&page, "%s %s %s:%d %s | reviewed %s | ID %s\n", state, f.Priority, f.File, f.Start, f.Explanation, f.Head, displayID(f.ID, terminalWidth()))
		for _, line := range responses[f.ID] {
			fmt.Fprintln(&page, line)
		}
	}
	if r.Repair != nil {
		fix := r.Repair
		fmt.Fprintf(&page, "Repair: %s | handback: %s | actor: %s\nWriter: %s | base: %s | commit: %s\n", fix.Status, fix.Handback, fix.Actor, fix.Writer, fix.Base, fix.Head)
		fmt.Fprintf(&page, "Selected: %d | fixer input: %s | rules: %s | config: %s\n", len(fix.Selected), fix.InputDigest, fix.Rules.Digest, fix.ConfigDigest)
		if fix.Confinement != nil {
			fmt.Fprintln(&page, confinementLine(fix.Confinement))
		}
		for _, check := range fix.Checks {
			fmt.Fprintf(&page, "Check %s exit=%d: %s\n", strings.Join(check.Args, " "), check.Exit, check.Status)
		}
		if fix.Log != "" {
			fmt.Fprintln(&page, fix.Log)
		}
	}
	if r.Review != nil {
		x := r.Review
		fmt.Fprintf(&page, "Review: %s | %s | %s\nInput: %s\nCriteria: %s\nConfig: %s\n%s\n", x.Status, x.Backend, x.Config.Model, x.InputDigest, x.Rules.Digest, x.ConfigDigest, x.Log)
		if x.Snapshot != nil {
			fmt.Fprintln(&page, snapshotLine(x.Snapshot))
		}
		if x.Confinement != nil {
			fmt.Fprintln(&page, confinementLine(x.Confinement))
		}
		if details {
			fmt.Fprintf(&page, "Executor: %s\nArgs: %s\n", x.Config.Executable, strings.Join(x.Args, " "))
		}
		if x.Result != nil {
			fmt.Fprintf(&page, "Reported model: %s/%s\nCoverage: %s\n", x.Result.ReportedProvider, x.Result.ReportedModel, strings.Join(x.Result.Coverage, ", "))
			for _, f := range x.Result.Findings {
				fmt.Fprintf(&page, "%s %s %s:%d-%d %s\nAction: %s\nRule: %s | ID: %s\n", f.Priority, f.Origin, f.File, f.Start, f.End, f.Explanation, f.Action, f.Rule, displayID(f.ID, terminalWidth()))
			}
		}
	}
	return page.String()
}
func screen(text string) {
	fmt.Print("\033[2J\033[H")
	fmt.Println(terminalText(text))
}
func terminalPrompt(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	fmt.Print(terminalText(text))
	if strings.HasSuffix(text, " ") {
		fmt.Print(" ")
	}
}
func lineTerminal(h, path string) error {
	q := engine.Request{Version: engine.MaxVersion, Op: "attach", Path: path}
	v, e := call(h, q)
	if e != nil {
		return e
	}
	if e = protocolRefusal(v); e != nil {
		return e
	}
	lines := make(chan string)
	go func() {
		reader := bufio.NewScanner(os.Stdin)
		for reader.Scan() {
			lines <- reader.Text()
		}
		close(lines)
	}()
	input := ""
	scan := func() bool {
		line, ok := <-lines
		input = line
		return ok
	}
	updates := make(chan liveUpdate, 1)
	stop := make(chan struct{})
	defer close(stop)
	go follow(h, path, v.Stream, v.Sequence, updates, stop)
	live := "live"
	message := ""
	details := false
	order := snapshotOrder{}
	for {
		if v.Run == nil {
			return errors.New(v.Error)
		}
		r := v.Run
		s := r.Configs[len(r.Configs)-1]
		var page strings.Builder
		page.WriteString(statePage(v, live, details))
		fmt.Fprintln(&page, "\n"+wrapMenuEntries([]string{"[i] Inspect", "[c] Criteria/diff", "[l] Load rules", "[s] Settings"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[p] Intent/target", "[v] Review", "[g] Pipeline", "[d] Decide"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[m] Resolve conflict", "[f] Selected repair", "[h] Handback"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[r] Rerun", "[a] Cancel auto-resume", "[b] Rerun from start"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[t] Trust repository", "[u] Migrate config", "[o] Logs"}, terminalWidth())+"\n"+
			wrapMenuEntries([]string{"[x] Cancel", "[n] New run", "[e] Recover", "[q] Detach"}, terminalWidth()))
		fmt.Fprint(&page, message)
		screen(page.String())
		terminalPrompt("> ")
		op, entered, redraw := "", false, false
		for !redraw && !entered {
			select {
			case u := <-updates:
				if u.view != nil && u.view.Run != nil && order.accepts(*u.view, v, false) {
					order.adopt(*u.view, v)
					v, redraw = *u.view, true
				}
				if u.state != live {
					live, redraw = u.state, true
				}
			case line, ok := <-lines:
				if !ok {
					return nil
				}
				op, entered = line, true
			}
		}
		if !entered {
			continue
		}
		if op == "q" {
			return nil
		}
		q = engine.Request{Version: engine.MaxVersion, Op: "inspect", Path: path}
		if op == "o" {
			logged, err := call(h, engine.Request{Version: engine.MaxVersion, Op: "transcript", Path: path, RunID: r.ID})
			if err != nil {
				return err
			}
			if logged.Transcript == nil {
				message = logged.Outcome + " " + logged.Error
				continue
			}
			screen(transcriptPage(r.ID, logged.Transcript))
			terminalPrompt("Press Enter to return\n> ")
			if !scan() {
				return nil
			}
			continue
		}
		if op == "c" {
			details = !details
		}
		if op == "v" {
			q.Op = "review"
		}
		if op == "x" {
			q.Op = "cancel"
		}
		if op == "f" {
			terminalPrompt("Finding IDs as shown (separated by commas or spaces):\n> ")
			if !scan() {
				return nil
			}
			var reviewed []string
			if r.Review != nil && r.Review.Result != nil {
				reviewed = engine.FindingIDs(r.Review.Result.Findings)
			}
			selected, whole, err := engine.ParseSelection(input, reviewed)
			if whole {
				err = errors.New("no finding ID entered")
			}
			if err != nil {
				message = err.Error() + "; nothing submitted"
				continue
			}
			q.Selected = selected
			q.Op = "repair"
		}
		if op == "h" {
			q.Op = "handback"
		}
		if op == "d" {
			if v.Decision == nil {
				message = "No open decision"
				continue
			}
			terminalPrompt("Choice (%s):\n> ", strings.Join(v.Decision.Options, ", "))
			if !scan() {
				return nil
			}
			q.Choice = input
			if !slices.Contains(v.Decision.Options, q.Choice) {
				message = "unknown choice " + strconv.Quote(q.Choice) + "; enter an option exactly as listed; nothing submitted"
				continue
			}
			if q.Choice == "repair" && v.Decision.Kind == "findings" {
				terminalPrompt("Finding IDs as shown (separated by commas or spaces; an empty line selects the whole group):\n> ")
				if !scan() {
					return nil
				}
				selected, _, err := engine.ParseSelection(input, v.Decision.Findings)
				if err != nil {
					message = err.Error() + "; nothing submitted"
					continue
				}
				q.Selected = selected
			}
			if q.Choice == "increase-cap" {
				terminalPrompt("New limit:\n> ")
				if !scan() {
					return nil
				}
				if q.Limit, e = engine.ParseCount(input); e != nil {
					message = e.Error() + "; nothing submitted"
					continue
				}
			}
			terminalPrompt("Reason:\n> ")
			if !scan() {
				return nil
			}
			q.Reason = input
			q.Op = "decide"
		}
		if op == "e" {
			if r.Recovery == nil {
				message = "No open recovery"
				continue
			}
			terminalPrompt("Recovery (%s):\n> ", strings.Join(r.Recovery.Options, ", "))
			if !scan() {
				return nil
			}
			q.Choice = input
			if !slices.Contains(r.Recovery.Options, q.Choice) {
				message = "unknown choice " + strconv.Quote(q.Choice) + "; enter an option exactly as listed; nothing submitted"
				continue
			}
			terminalPrompt("Reason:\n> ")
			if !scan() {
				return nil
			}
			q.Reason = input
			q.Op = "recover"
		}
		if op == "g" {
			q.Op = "pipeline"
		}
		if op == "m" {
			q.Op = "resolve-conflict"
		}
		if op == "p" {
			agents := engine.InstalledAgents()
			if len(agents) == 0 {
				message = "No supported agent installed. Install pi, claude or codex on PATH and use its native login."
				continue
			}
			terminalPrompt("Installed agents: %s\nReviewer agent [%s] (Enter keeps current):\n> ", strings.Join(agents, ", "), s.Settings.ReviewerBackend)
			if !scan() {
				return nil
			}
			if input != "" && input != s.Settings.ReviewerBackend {
				if !slices.Contains(agents, input) {
					message = "Choose an installed agent: " + strings.Join(agents, ", ")
					continue
				}
				draft := s.Settings
				draft.ReviewerBackend = input
				draft.FixerBackend = input
				draft.ReviewerModel = ""
				draft.FixerModel = ""
				applied, err := call(h, engine.Request{Version: engine.MaxVersion, Op: "apply", Path: path, RunID: r.ID, Input: r.Input, Gate: v.Gate, Expected: r.Token, Action: engine.TerminalMutation(v, "apply").Action, Reason: "select installed agent", Settings: &draft})
				if err != nil {
					return err
				}
				accepted := order.accepts(applied, v, true)
				if accepted {
					v = applied
				}
				if applied.Outcome != "ok" {
					message = applied.Error
					continue
				}
				if !accepted {
					message = "Agent selection confirmed; waiting for follower state. Submit intent after resync."
					continue
				}
				r = v.Run
				q.RunID = r.ID
				q.Input = r.Input
				q.Expected = r.Token
				q.Gate = v.Gate
			}
			terminalPrompt("Intent:\n> ")
			if !scan() {
				return nil
			}
			q.Intent = input
			suggestion := repository.SuggestTarget(path)
			terminalPrompt("Review committed changes against Git target [%s]:\n> ", suggestion)
			if !scan() {
				return nil
			}
			q.Target = input
			if q.Target == "" {
				q.Target = suggestion
			}
			q.Op = "prepare"
		}
		if op == "l" {
			q.Op = "resolve-rules"
		}
		if op == "r" {
			q.Op = "rerun"
		}
		if op == "a" {
			q.Op = "cancel-auto-resume"
		}
		if op == "s" {
			models, modelErr := call(h, engine.Request{Version: engine.MaxVersion, Op: "models", Path: path})
			modelsText := strings.Join(models.Models, ", ")
			if modelErr != nil {
				modelsText = "catalog unavailable: " + modelErr.Error()
			}
			terminalPrompt("Current values and sources:\n%s\nSettings draft (current %s %s %d)\nEnter mode preset limit, e.g. ask one-pass 6:\n> ", engine.SettingsTable(s), s.Settings.Mode, s.Settings.Preset, s.Settings.Limit)
			if !scan() {
				return nil
			}
			draft := s.Settings
			if draft.Mode, draft.Preset, draft.Limit, e = engine.ParseDraft(input); e != nil {
				message = "Invalid draft: " + e.Error()
				continue
			}
			terminalPrompt("Reviewer backend (pi, claude, codex) [%s]:\n> ", draft.ReviewerBackend)
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.ReviewerBackend = value
			}
			terminalPrompt("Pi models: %s\nEnter default to use the native model. Claude and Codex also accept an exact model ID.\nReviewer model [%s]:\n> ", modelsText, draft.ReviewerModel)
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.ReviewerModel = value
				if value == "default" {
					draft.ReviewerModel = ""
				}
			}
			terminalPrompt("Fixer backend (pi, claude, codex) [%s]:\n> ", draft.FixerBackend)
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.FixerBackend = value
			}
			terminalPrompt("Pi models: %s\nFixer model [%s]:\n> ", modelsText, draft.FixerModel)
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.FixerModel = value
				if value == "default" {
					draft.FixerModel = ""
				}
			}
			terminalPrompt("Selected rules (comma-separated; ? means optional; file:/absolute/path):\n> ")
			if !scan() {
				return nil
			}
			draft.Rules = []string{}
			for _, rule := range strings.Split(input, ",") {
				if name := strings.TrimSpace(rule); name != "" {
					draft.Rules = append(draft.Rules, name)
				}
			}
			terminalPrompt("Auto-resume (on, off; empty keeps %s):\n> ", engine.AutoResumeLabel(draft))
			if !scan() {
				return nil
			}
			if value := input; value != "" {
				draft.AutoResume = value
			}
			terminalPrompt("[a] Apply to run  [d] Save user defaults\n[r] Save repository override  [c] Cancel\n> ")
			if !scan() {
				return nil
			}
			switch input {
			case "a":
				q.Op = "apply"
			case "d":
				q.Op = "save"
			case "r":
				q.Op, q.Target = "save", "repository"
			default:
				continue
			}
			q.Settings = &draft
			q.Reason = "terminal Settings"
		} else if op == "t" {
			o := v.Repository
			if o == nil {
				continue
			}
			terminalPrompt("Trust %s of this repository (digest %.12s)? Its values apply to new runs. Type trust:\n> ", ".greenhouse.json", o.Digest)
			if !scan() {
				return nil
			}
			if input != "trust" {
				continue
			}
			q.Op, q.Digest, q.Reason = "trust", o.Digest, "terminal trust"
		} else if op == "u" {
			terminalPrompt("Migrate older configuration files? Each keeps a backup. Type migrate:\n> ")
			if !scan() {
				return nil
			}
			if input != "migrate" {
				continue
			}
			q.Op, q.Reason = "migrate", "terminal migrate"
		} else if op == "b" {
			terminalPrompt("Keep this run and start a linked rerun from start? Type rerun:\n> ")
			if !scan() {
				return nil
			}
			if input != "rerun" {
				continue
			}
			q.Op = "rerun-from-start"
		} else if op == "n" {
			terminalPrompt("Retain evidence and start New run? Type new:\n> ")
			if !scan() {
				return nil
			}
			if input != "new" {
				continue
			}
			q.Op = "new"
		}
		if q.Op != "inspect" {
			q.Action = engine.TerminalMutation(v, q.Op).Action
			q.Client = "terminal"
			q.Expected = v.Token
			q.RunID = v.RunID
			q.Input = v.Input
			q.Gate = v.Gate
		}
		answer, e := call(h, q)
		if e != nil {
			return e
		}
		if order.accepts(answer, v, true) {
			v = answer
		}
		message = answer.Outcome + " " + answer.Error
	}
}

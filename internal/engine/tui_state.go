package engine

import (
	"fmt"
	"strings"
)

// tuiAction is one thing the terminal can offer. Op names the controller
// operation it eventually sends; an empty Op opens a view and sends nothing.
type tuiAction struct {
	ID      string
	Op      string
	Label   string
	Hint    string
	Confirm string
}

var tuiActions = map[string]tuiAction{
	"run":                {ID: "run", Op: "prepare", Label: "Run review", Hint: "Collect the intent from your agent session, load rules and start one independent review. It uses one round of the budget."},
	"setup":              {ID: "setup", Op: "prepare", Label: "Change agent, intent or target", Hint: "Edit what Run uses. Nothing is sent until you Run."},
	"resolve-rules":      {ID: "resolve-rules", Op: "resolve-rules", Label: "Load rules", Hint: "Snapshot the selected rule packs for this run. Nothing runs yet."},
	"fix-rules":          {ID: "fix-rules", Label: "Fix rule selection", Hint: "A required rule failed to load. Change the rules in Settings, then load them again."},
	"review":             {ID: "review", Op: "review", Label: "Start review", Hint: "Dispatch one independent review. It uses one round of the budget."},
	"pipeline":           {ID: "pipeline", Op: "pipeline", Label: "Start pipeline", Hint: "Run the trusted pipeline stages in order on a private candidate."},
	"decide":             {ID: "decide", Op: "decide", Label: "Answer decision", Hint: "The pipeline waits for your answer."},
	"recover":            {ID: "recover", Op: "recover", Label: "Resolve recovery", Hint: "An execution outcome is unknown. Inspect the evidence and choose an explicit recovery."},
	"resolve-conflict":   {ID: "resolve-conflict", Op: "resolve-conflict", Label: "Let the fixer resolve the conflict", Hint: "Authorize the fixer to edit only the conflicted paths and commit the merge."},
	"findings":           {ID: "findings", Label: "Open findings", Hint: "Read each finding with its location and full description."},
	"repair":             {ID: "repair", Op: "repair", Label: "Repair selected findings", Hint: "Select findings, then let the trusted fixer repair them in a private worktree."},
	"handback":           {ID: "handback", Op: "handback", Label: "Hand back the fix", Hint: "Fast-forward your unchanged branch to the repaired commit."},
	"rerun":              {ID: "rerun", Op: "rerun", Label: "Rerun stage", Hint: "Run the stopped stage again with the current settings. Earlier evidence stays."},
	"cancel-auto-resume": {ID: "cancel-auto-resume", Op: "cancel-auto-resume", Label: "Cancel automatic resume", Hint: "Keep the run paused until you resume it."},
	"cancel":             {ID: "cancel", Op: "cancel", Label: "Cancel running work", Hint: "Stop the running review, fixer, check or pipeline step. The run stays rerunnable.", Confirm: "x"},
	"rerun-from-start":   {ID: "rerun-from-start", Op: "rerun-from-start", Label: "Rerun from start", Hint: "Keep this run and start a linked run with the same settings, intent and target.", Confirm: "rerun"},
	"new":                {ID: "new", Op: "new", Label: "Start a new run", Hint: "Retain this run's evidence and start a new run for the current head.", Confirm: "new"},
	"migrate":            {ID: "migrate", Op: "migrate", Label: "Migrate configuration", Hint: "Upgrade older configuration files. Each keeps a backup.", Confirm: "migrate"},
	"trust":              {ID: "trust", Op: "trust", Label: "Trust repository override", Hint: "Apply this repository's .greenhouse.json to new runs. The current run does not change.", Confirm: "trust"},
	"settings":           {ID: "settings", Label: "Settings", Hint: "Draft mode, preset, budget, agents, models and rules; Apply or Save explicitly."},
	"details":            {ID: "details", Label: "Details", Hint: "Run IDs, digests, sources, rules, criteria and the full controller state."},
	"logs":               {ID: "logs", Label: "Transcript log", Hint: "The last lines of the redacted reviewer and fixer transcript."},
	"help":               {ID: "help", Label: "Help", Hint: "Keys and actions, including ones unavailable now."},
}

func busy(r *Run) bool {
	return r != nil && (r.Status == "running" || r.Status == "cancelling" || r.Status == "writing")
}

// followOnly reports a run the controller lets this client watch but not
// change, such as a run owned by a host.
func followOnly(v Response) bool {
	for _, op := range v.Actions {
		if op != "attach" && op != "inspect" {
			return false
		}
	}
	return true
}

func rulesFailed(v Response) bool {
	for _, rule := range v.Rules {
		if rule.Required && rule.Status == "failed" {
			return true
		}
	}
	return false
}

func labelled(id string, r *Run) tuiAction {
	a := tuiActions[id]
	if id == "rerun" && r != nil && r.Pause != nil {
		a.Label, a.Hint = "Resume paused stage", "Run the paused stage again in a fresh session with the same input."
	}
	if id == "run" && r != nil && r.Configs[len(r.Configs)-1].Settings.Preset == "full" {
		a.Label, a.Hint = "Run pipeline", "Collect the intent from your agent session, load rules and start the trusted pipeline."
	}
	if id == "run" && r != nil && r.Intent != "" {
		a.Hint = strings.Replace(a.Hint, "Collect the intent from your agent session, load", "Use the saved intent, load", 1)
	}
	if id == "pipeline" && r != nil && r.Pipeline != nil && pipelineStarted(r.Pipeline) {
		a.Label = "Continue pipeline"
	}
	return a
}

func pipelineStarted(p *Pipeline) bool {
	for _, st := range p.Stages {
		if st.Status != "pending" {
			return true
		}
	}
	return false
}

// primaryAction picks the one next step Enter runs on the home screen. It is
// derived from the controller's allowed actions and run state only.
func primaryAction(v Response, agents []string) tuiAction {
	r := v.Run
	if r == nil || busy(r) || followOnly(v) {
		return tuiAction{}
	}
	allowed := func(op string) bool { return has(v.Actions, op) }
	actionable, _ := outstanding(r)
	switch {
	case r.Recovery != nil && allowed("recover"):
		return labelled("recover", r)
	case v.Decision != nil && allowed("decide"):
		return labelled("decide", r)
	case allowed("resolve-conflict"):
		return labelled("resolve-conflict", r)
	case allowed("migrate"):
		return labelled("migrate", r)
	case r.Pause != nil && allowed("rerun"):
		return labelled("rerun", r)
	case !v.RulesReady && rulesFailed(v) && allowed("apply"):
		return labelled("fix-rules", r)
	case allowed("prepare"):
		limit := r.Configs[len(r.Configs)-1].Settings.Limit
		if len(agents) == 0 && r.Reviewer == nil || r.ReviewUsed+r.RepairUsed >= limit {
			return tuiAction{}
		}
		return labelled("run", r)
	case !v.RulesReady && rulesFailed(v) && allowed("apply"):
		return labelled("fix-rules", r)
	case allowed("handback"):
		return labelled("handback", r)
	case !v.RulesReady && allowed("resolve-rules") && r.Repair == nil:
		return labelled("resolve-rules", r)
	case allowed("review"):
		return labelled("review", r)
	case allowed("pipeline"):
		return labelled("pipeline", r)
	case r.Status == "outstanding-findings" && len(actionable) > 0:
		return labelled("findings", r)
	case allowed("rerun"):
		return labelled("rerun", r)
	case allowed("new") && !allowed("apply"):
		return labelled("new", r)
	}
	return tuiAction{}
}

// menuItems lists the actions menu. Mutations appear only when the
// controller allows their operation and the client holds fresh state.
func menuItems(v Response, fresh bool) []tuiAction {
	r := v.Run
	var items []tuiAction
	if r == nil {
		return items
	}
	allowed := func(op string) bool { return fresh && has(v.Actions, op) }
	add := func(id string, ok bool) {
		if ok {
			items = append(items, labelled(id, r))
		}
	}
	add("run", allowed("prepare"))
	add("setup", allowed("prepare"))
	add("resolve-rules", allowed("resolve-rules"))
	add("review", allowed("review"))
	add("pipeline", allowed("pipeline"))
	add("decide", allowed("decide") && v.Decision != nil)
	add("resolve-conflict", allowed("resolve-conflict"))
	add("repair", allowed("repair"))
	add("handback", allowed("handback"))
	add("rerun", allowed("rerun"))
	add("cancel-auto-resume", allowed("cancel-auto-resume") && r.Pause != nil)
	add("cancel", allowed("cancel"))
	add("recover", allowed("recover") && r.Recovery != nil)
	add("migrate", allowed("migrate"))
	add("trust", fresh && !followOnly(v) && trustable(v))
	add("rerun-from-start", allowed("rerun-from-start"))
	add("new", allowed("new"))
	add("settings", true)
	add("findings", len(r.Findings) > 0)
	add("details", true)
	add("logs", true)
	add("help", true)
	return items
}

// trustable reports an override file the repository status shows as present
// but not applied; the controller still checks its digest.
func trustable(v Response) bool {
	o := v.Repository
	return o != nil && o.Status == "ignored" && o.Digest != "absent" && o.Digest != ""
}

func mutation(v Response, op string) Request {
	return Request{Version: protocolMax, Op: op, RunID: v.RunID, Input: v.Input, Gate: v.Gate, Expected: v.Token, Action: opaque(), Client: "terminal"}
}

type tuiStep struct {
	Name  string
	State string
	Text  string
}

const (
	stepDone    = "done"
	stepTodo    = "todo"
	stepFailed  = "failed"
	stepRunning = "running"
	stepWaiting = "waiting"
)

func agentLabel(backend, model string) string {
	name := backendNames[backend]
	if name == "" {
		name = backend
	}
	if model == "" {
		return name + " · native default model"
	}
	return name + " · " + model
}

func setupSteps(v Response, agents []string, suggestion string) []tuiStep {
	r := v.Run
	settings := r.Configs[len(r.Configs)-1].Settings
	var steps []tuiStep
	switch {
	case r.Reviewer != nil:
		steps = append(steps, tuiStep{"Agent", stepDone, agentLabel(backendOf(*r.Reviewer), r.Reviewer.Model)})
	case len(agents) == 0:
		steps = append(steps, tuiStep{"Agent", stepFailed, "none found: install pi, claude or codex"})
	case has(agents, settings.ReviewerBackend):
		steps = append(steps, tuiStep{"Agent", stepTodo, agentLabel(settings.ReviewerBackend, settings.ReviewerModel) + " (installed)"})
	default:
		steps = append(steps, tuiStep{"Agent", stepTodo, "installed: " + strings.Join(agents, ", ")})
	}
	if r.Intent != "" {
		steps = append(steps, tuiStep{"Intent", stepDone, intentSummary(r.Intent)})
	} else {
		steps = append(steps, tuiStep{"Intent", stepTodo, "describe the change"})
	}
	switch {
	case r.Target != "":
		steps = append(steps, tuiStep{"Target", stepDone, targetLabel(r.Target)})
	case suggestion != "":
		steps = append(steps, tuiStep{"Target", stepTodo, "suggested " + targetLabel(suggestion)})
	default:
		steps = append(steps, tuiStep{"Target", stepTodo, "choose a Git target"})
	}
	names := strings.Join(settings.Rules, ", ")
	if names == "" {
		names = "none selected"
	}
	switch {
	case !v.RulesReady && r.Review != nil:
		steps = append(steps, tuiStep{"Rules", stepDone, names + " · used by the review"})
	case v.RulesReady && len(v.Rules) > 0 && v.Rules[0].Status != "configured":
		steps = append(steps, tuiStep{"Rules", stepDone, names + " · loaded"})
	case v.RulesReady:
		steps = append(steps, tuiStep{"Rules", stepDone, names})
	case rulesFailed(v):
		steps = append(steps, tuiStep{"Rules", stepFailed, names + " · failed to load"})
	default:
		steps = append(steps, tuiStep{"Rules", stepTodo, names + " · not loaded"})
	}
	steps = append(steps, reviewStep(v))
	return steps
}

func targetLabel(target string) string {
	if len(target) == 40 || len(target) == 64 {
		return target[:7]
	}
	return target
}

func reviewStep(v Response) tuiStep {
	r := v.Run
	limit := r.Configs[len(r.Configs)-1].Settings.Limit
	budget := fmt.Sprintf("budget %d of %d used", r.ReviewUsed+r.RepairUsed, limit)
	name := "Review"
	if r.Pipeline != nil || r.Configs[len(r.Configs)-1].Settings.Preset == "full" {
		name = "Pipeline"
	}
	actionable, informational := outstanding(r)
	switch {
	case r.Recovery != nil:
		return tuiStep{name, stepFailed, "recovery required · " + budget}
	case r.Pause != nil:
		return tuiStep{name, stepWaiting, "paused · " + budget}
	case busy(r):
		return tuiStep{name, stepRunning, statusWords(r.Status) + " · " + budget}
	case v.Decision != nil:
		return tuiStep{name, stepWaiting, "decision required · " + budget}
	case r.Pipeline != nil && r.Pipeline.Outcome != "" && (r.Pipeline.Status == "ci-ready" || r.Pipeline.Status == "locally-ready"):
		state := stepDone
		if r.Pipeline.Outcome == "accepted-risk" {
			state = stepWaiting
		}
		return tuiStep{name, state, strings.ToLower(outcomeLabels[r.Pipeline.Outcome]) + " · " + r.Pipeline.Status}
	case r.Pipeline != nil && r.Pipeline.Failed != "":
		return tuiStep{name, stepFailed, "stopped at " + r.Pipeline.Failed + " · " + budget}
	case r.Status == "clean-review":
		return tuiStep{name, stepDone, "clean · " + budget}
	case r.Status == "outstanding-findings":
		return tuiStep{name, stepFailed, findingCount(len(actionable), len(informational)) + " · " + budget}
	case r.Status == "unconfigured":
		return tuiStep{name, stepTodo, "not started · " + budget}
	}
	return tuiStep{name, stepWaiting, statusWords(r.Status) + " · " + budget}
}

func findingCount(actionable, informational int) string {
	text := plural(actionable, "outstanding finding")
	if informational > 0 {
		text += fmt.Sprintf(", %d informational", informational)
	}
	return text
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func statusWords(status string) string {
	return strings.ReplaceAll(status, "-", " ")
}

// stateNotes explains the current state in plain words from structured
// fields, so the home screen does not depend on blocker strings.
func stateNotes(v Response, agents []string, fresh bool) []string {
	r := v.Run
	var notes []string
	if r == nil {
		return notes
	}
	if !fresh {
		notes = append(notes, "Reconnecting to the Greenhouse daemon. The last state stays on screen; actions are disabled until fresh state arrives.")
	}
	if followOnly(v) {
		if r.Host != nil {
			notes = append(notes, "This run is owned by "+hostLabel(r.Host)+". You can follow it here but not change it.")
		} else {
			notes = append(notes, "This run can be followed but not changed from here.")
		}
		return notes
	}
	if r.Reviewer == nil && len(agents) == 0 && has(v.Actions, "prepare") {
		notes = append(notes, "No supported agent found on PATH. Install Pi, Claude Code or Codex, sign in with its own CLI, then press r to recheck.")
	}
	for _, c := range v.Config {
		if c.Status == "older" || c.Status == "newer" || c.Status == "invalid" {
			notes = append(notes, "Configuration "+c.File+" is "+c.Status+": "+c.Error)
		}
	}
	handedBack := r.Repair != nil && r.Repair.Handback == "handed-back" || r.Pipeline != nil && r.Pipeline.Handback == "handed-back"
	switch {
	case handedBack && !has(v.Actions, "apply"):
		notes = append(notes, "The fix is on your branch now. Start a new run to review it independently.")
	case has(v.Actions, "new") && !has(v.Actions, "apply") && !busy(r) && r.Recovery == nil:
		notes = append(notes, "Your branch or head moved since this run started. Start a new run to review the current head.")
	}
	if r.Pause != nil && v.Explanation != "" {
		explanation, _, _ := strings.Cut(v.Explanation, " Press ")
		if has(v.Actions, "rerun") {
			explanation += " Enter resumes now."
		}
		if has(v.Actions, "cancel-auto-resume") {
			explanation += " The actions menu (a) cancels the automatic resume."
		}
		notes = append(notes, explanation+" Settings (s) changes the agent.")
	}
	if rec := r.Recovery; rec != nil {
		notes = append(notes, "Unknown "+rec.Kind+" outcome at "+rec.Stage+": "+rec.Reason)
	}
	if d := v.Decision; d != nil {
		notes = append(notes, "Decision required ("+d.Kind+"): "+d.Reason)
	}
	if p := r.Pipeline; p != nil && p.Failed != "" && p.Status != "running" && v.Decision == nil {
		notes = append(notes, "Pipeline stopped at "+p.Failed+": "+p.Reason)
	}
	if p := r.Pipeline; p != nil && len(p.Conflict) > 0 {
		notes = append(notes, "Target conflict in "+strings.Join(p.Conflict, ", ")+".")
	}
	limit := r.Configs[len(r.Configs)-1].Settings.Limit
	finished := r.Status == "clean-review" || r.Pipeline != nil && (r.Pipeline.Status == "ci-ready" || r.Pipeline.Status == "locally-ready") || handedBack
	if r.Host == nil && r.ReviewUsed+r.RepairUsed >= limit && !finished {
		notes = append(notes, fmt.Sprintf("Budget exhausted: %d of %d rounds used. Raise the limit in Settings to continue.", r.ReviewUsed+r.RepairUsed, limit))
	}
	actionable, _ := outstanding(r)
	if r.Status == "outstanding-findings" && len(actionable) > 0 && r.Repair == nil && r.RepairUsed == 0 && !has(v.Actions, "repair") && r.Host == nil {
		notes = append(notes, "Selected repair is unavailable: it needs a trusted fixer.json with checks and findings on changed files.")
	}
	if x := r.Repair; x != nil && x.Status == "fixed-not-independently-rereviewed" {
		notes = append(notes, "The fix passed its checks but was not independently reviewed again.")
	}
	if p := r.Pipeline; p != nil && p.HandbackError != "" {
		notes = append(notes, "Handback refused: "+p.HandbackError)
	}
	if trustable(v) {
		notes = append(notes, overrideLine(v.Repository)+".")
	}
	return notes
}

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleDim     = lipgloss.NewStyle().Faint(true)
	styleFocus   = lipgloss.NewStyle().Reverse(true).Bold(true)
	styleDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleWaiting = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

func fit(line string, width int) string {
	if ansi.StringWidth(line) <= width {
		return line
	}
	return ansi.Truncate(line, width, "…")
}

func wrapIndent(text string, width int, indent string) []string {
	lines := wrap(text, width-len(indent))
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return lines
}

func wrap(text string, width int) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		clean := strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return ' '
			}
			return r
		}, line)
		out = append(out, strings.Split(ansi.Wrap(clean, width, ""), "\n")...)
	}
	return out
}

func (m *tuiModel) View() string {
	w, h := m.width, m.height
	if w < 40 || h < 12 {
		return fit("Greenhouse needs at least 40x12; resize the terminal.", max(w, 1))
	}
	header := m.header(w)
	footer := []string{m.footer(w)}
	if m.message != "" {
		style := styleDim
		if m.failed {
			style = styleFailed
		}
		lines := wrap(m.message, w-1)
		if len(lines) > 3 {
			lines = append(lines[:2], lines[2]+" …")
		}
		for i := len(lines) - 1; i >= 0; i-- {
			footer = append([]string{style.Render(fit(lines[i], w-1))}, footer...)
		}
	}
	room := h - len(header) - len(footer)
	body := m.body(w-1, room)
	if len(body) > room {
		body = body[:room]
	}
	for len(body) < room {
		body = append(body, "")
	}
	lines := append(append(header, body...), footer...)
	for i, line := range lines {
		lines[i] = fit(line, w-1)
	}
	return strings.Join(lines, "\n")
}

func (m *tuiModel) header(w int) []string {
	r := m.v.Run
	branch := strings.TrimPrefix(r.Submitted.Branch, "refs/heads/")
	if branch == "" {
		branch = "detached"
	}
	left := styleBold.Render("Greenhouse") + " · " + branch + " @ " + targetLabel(r.Submitted.Head)
	settings := r.Configs[len(r.Configs)-1].Settings
	right := settings.Preset + " · " + settings.Mode
	if m.live == "live" {
		right += " · live"
	} else {
		right = styleFailed.Render("RECONNECTING")
	}
	gap := w - 1 - ansi.StringWidth(left) - ansi.StringWidth(right)
	line := left + strings.Repeat(" ", max(gap, 1)) + right
	if gap < 1 {
		line = left
	}
	return []string{line, styleDim.Render(strings.Repeat("─", w-1))}
}

func (m *tuiModel) footer(w int) string {
	var keys []string
	switch m.screen {
	case screenHome:
		targets := homeTargets(m)
		if m.focus < len(targets) && targets[m.focus].label != "" && m.seq == nil {
			keys = append(keys, "⏎ "+targets[m.focus].label)
		}
		if m.seq != nil {
			keys = append(keys, "x stop run")
		}
		if has(m.v.Actions, "cancel") {
			keys = append(keys, "x cancel")
		}
		keys = append(keys, "a actions", "s settings")
		if m.v.Run != nil && len(m.v.Run.Findings) > 0 {
			keys = append(keys, "f findings")
		}
		if !m.fresh {
			keys = append(keys, "r reconnect")
		}
		keys = append(keys, "? help", "i details")
	case screenMenu:
		keys = []string{"⏎ choose", "↑↓ move", "esc back"}
	case screenSetup:
		keys = []string{"⏎ next", "esc back"}
		if m.setupStep == 0 {
			keys = []string{"⏎ choose", "↑↓ move", "esc back"}
		}
		if m.setupStep == 3 {
			keys = []string{"⏎ save", "esc back"}
		}
	case screenFindings:
		keys = []string{"↑↓ move", "⏎ details"}
		if has(m.v.Actions, "repair") {
			keys = append(keys, "space select", "r repair selected")
		}
		keys = append(keys, "esc back")
	case screenIntent:
		keys = []string{"⏎ choose", "↑↓ move", "esc stop run"}
	case screenDecision, screenRecovery:
		keys = []string{"⏎ next", "↑↓ move", "esc back"}
		if m.decision.step == 1 && m.screen == screenDecision {
			keys = []string{"space select", "⏎ next", "esc back"}
		}
	case screenSettings:
		keys = []string{"↑↓ move", "←→ change", "⏎ edit or run", "esc back (keeps draft)"}
	case screenPage:
		keys = []string{"↑↓ scroll", "esc back"}
		if m.pageKind == "details" {
			keys = []string{"↑↓ scroll", "c criteria", "o logs", "esc back"}
		}
	case screenConfirm:
		switch m.confirm.Confirm {
		case "x":
			keys = []string{"x confirm cancel", "esc keep running"}
		case "":
			keys = []string{"⏎ confirm", "esc back"}
		default:
			keys = []string{"⏎ confirm", "esc back"}
		}
	}
	if m.editing && m.screen == screenIntent {
		keys = []string{"⏎ continue run", "esc stop run"}
	} else if m.editing {
		keys = []string{"⏎ accept", "esc back"}
	} else if busy(m.v.Run) {
		keys = append(keys, "q detach")
	} else {
		keys = append(keys, "q quit")
	}
	quit := keys[len(keys)-1]
	line := ""
	for _, k := range keys[:len(keys)-1] {
		next := k
		if line != "" {
			next = line + "  " + k
		}
		if ansi.StringWidth(next)+2+ansi.StringWidth(quit) > w-1 {
			break
		}
		line = next
	}
	if line != "" {
		line += "  "
	}
	return styleDim.Render(line + quit)
}

func (m *tuiModel) body(w, room int) []string {
	switch m.screen {
	case screenMenu:
		return m.menuView(w, room)
	case screenSetup:
		return m.setupView(w)
	case screenFindings:
		return m.findingsView(w, room)
	case screenDecision:
		return m.decisionView(w, room)
	case screenRecovery:
		return m.recoveryView(w, room)
	case screenSettings:
		return m.settingsView(w, room)
	case screenPage:
		return m.pageView(w, room)
	case screenConfirm:
		return m.confirmView(w)
	case screenIntent:
		return m.intentView(w, room)
	}
	return m.homeView(w, room)
}

func (m *tuiModel) icon(state string) string {
	switch state {
	case stepDone:
		return styleDone.Render("✓")
	case stepFailed:
		return styleFailed.Render("✗")
	case stepRunning:
		return styleRunning.Render(m.spin.View())
	case stepWaiting:
		return styleWaiting.Render("!")
	}
	return "○"
}

func (m *tuiModel) homeView(w, room int) []string {
	targets := homeTargets(m)
	focused := ""
	if m.focus < len(targets) {
		focused = targets[m.focus].step
	}
	var steps []string
	for _, s := range m.homeSteps() {
		row := fmt.Sprintf(" %s %-8s %s", m.icon(s.State), s.Name, s.Text)
		if s.Name == focused {
			row = styleFocus.Render(fit(fmt.Sprintf("▸%s %-8s %s", ansi.Strip(m.icon(s.State)), s.Name, s.Text), w))
		}
		steps = append(steps, fit(row, w))
	}
	var progress []string
	if m.seq != nil {
		label := runStepLabels[m.seq.step]
		if m.seq.stopped {
			label = "Stopping after this step"
		}
		progress = append(progress, "", fit(" "+styleRunning.Render(m.spin.View())+" "+label+"…", w))
	}
	if r := m.v.Run; busy(r) {
		progress = append(progress, "", fit(" "+styleRunning.Render(m.spin.View())+" "+m.progressLine(), w))
	}
	if p := m.v.Run.Pipeline; p != nil {
		progress = append(progress, wrapIndent(stageStrip(p), w, " ")...)
	}
	primary := m.primaryBlock(w, m.focus == 0)
	var notes []string
	for _, note := range stateNotes(m.v, m.agents, m.fresh) {
		notes = append(notes, wrapIndent(note, w, " ")...)
	}
	if m.draftChanged() {
		notes = append(notes, " Settings draft has changes that are not applied or saved (s).")
	}
	var findings []string
	if r := m.v.Run; len(r.Findings) > 0 && !busy(r) {
		actionable, informational := outstanding(r)
		findings = append(findings, "", styleBold.Render(" Findings")+" · "+findingCount(len(actionable), len(informational)))
		for _, f := range append(actionable, informational...) {
			findings = append(findings, fit("   "+findingRow(f), w))
		}
	}
	fixed := len(steps) + len(progress) + len(primary) + 1
	space := room - fixed
	if len(notes) > 0 {
		space--
	}
	if len(notes) > space {
		notes = notes[:max(space, 1)]
	}
	space -= len(notes)
	if len(findings) > space {
		if space >= 3 {
			findings = append(findings[:space-1], styleDim.Render("   … f opens all findings"))
		} else {
			findings = nil
		}
	}
	out := append([]string{}, steps...)
	out = append(out, progress...)
	out = append(out, findings...)
	if len(notes) > 0 {
		out = append(out, "")
		out = append(out, notes...)
	}
	for len(out)+len(primary) < room-1 {
		out = append(out, "")
	}
	out = append(out, "")
	return append(out, primary...)
}

func (m *tuiModel) primaryBlock(w int, focused bool) []string {
	a := primaryAction(m.v, m.agents)
	if m.seq != nil {
		return wrapIndent("Run is in progress. x stops it after the current step; q detaches.", w, " ")
	}
	if a.ID == "" {
		return wrapIndent(m.idlePrompt(), w, " ")
	}
	label := "▸ " + a.Label + " "
	if focused {
		label = styleFocus.Render(label)
	} else {
		label = styleBold.Render(label)
	}
	out := []string{" " + label}
	hint := wrapIndent(a.Hint, w, "   ")
	if len(hint) > 3 {
		hint = hint[:3]
	}
	for _, line := range hint {
		out = append(out, styleDim.Render(line))
	}
	return out
}

func (m *tuiModel) idlePrompt() string {
	r := m.v.Run
	switch {
	case !m.fresh:
		return "Waiting for the daemon. Press r to reconnect."
	case followOnly(m.v):
		return "Following this run. Press i for details."
	case busy(r):
		return "Work runs in the daemon. x cancels it; q detaches and leaves it running."
	case r.Reviewer == nil && len(m.agents) == 0 && has(m.v.Actions, "prepare"):
		return "Next: install an agent and sign in with its own CLI, then press r."
	case r.Status == "clean-review":
		return "Review complete: clean for this head. a lists further actions."
	case r.Pipeline != nil && (r.Pipeline.Status == "ci-ready" || r.Pipeline.Status == "locally-ready"):
		return "Pipeline complete: " + r.Pipeline.Status + ". a lists further actions."
	}
	return "Nothing to start right now. a lists available actions; ? explains the rest."
}

func progressLabel(r *Run) string {
	switch {
	case r.Status == "cancelling":
		return "cancellation"
	case r.Pipeline != nil && r.Pipeline.Active != nil:
		return "the pipeline " + r.Pipeline.Active.Stage + " stage"
	case r.Status == "writing":
		return "the selected repair"
	}
	return "the independent review"
}

func (m *tuiModel) progressLine() string {
	r := m.v.Run
	stage, agent := "Independent review", ""
	if r.Review != nil {
		agent = agentName(backendOf(r.Review.Config), r.Review.Config.Model)
	}
	switch {
	case r.Status == "cancelling":
		stage = "Cancelling"
	case r.Pipeline != nil && r.Pipeline.Active != nil:
		a := r.Pipeline.Active
		stage = "Pipeline · " + a.Stage
		switch a.Kind {
		case "repair", "conflict":
			if n := len(r.Pipeline.Repairs); n > 0 {
				agent = agentName(r.Pipeline.Repairs[n-1].Config.Backend, r.Pipeline.Repairs[n-1].Config.Model)
			}
		case "review":
		default:
			agent = "trusted commands"
		}
	case r.Pipeline != nil:
		stage, agent = "Pipeline · scheduling", ""
	case r.Status == "writing" && r.Repair != nil:
		stage = "Selected repair · " + statusWords(r.Repair.Status)
		agent = agentName(r.Repair.Config.Backend, r.Repair.Config.Model)
	}
	elapsed := m.now().Sub(m.runSince).Round(time.Second)
	clock := formatElapsed(elapsed)
	if !m.runFromTop {
		clock = "≥" + clock
	}
	parts := []string{stage}
	if agent != "" {
		parts = append(parts, agent)
	}
	return strings.Join(append(parts, clock), " · ")
}

func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

var stageIcons = map[string]string{"passed": "✓", "failed": "✗", "running": "…", "pending": "○", "needs-decision": "!", "not-applicable": "-", "accepted-risk": "!", "repaired-unreviewed": "!", "unknown-outcome": "?", "paused": "!", "cancelled": "✗"}

func stageStrip(p *Pipeline) string {
	var parts []string
	for _, st := range p.Stages {
		icon := stageIcons[st.Status]
		if icon == "" {
			icon = "?"
		}
		parts = append(parts, st.Name+" "+icon)
	}
	return "Stages: " + strings.Join(parts, "  ")
}

func findingRow(f Finding) string {
	return fmt.Sprintf("%s %s:%d  %s", f.Priority, f.File, f.Start, f.Explanation)
}

func (m *tuiModel) list(rows []string, cursor, room int) []string {
	start := 0
	if cursor >= room {
		start = cursor - room + 1
	}
	end := min(len(rows), start+room)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, rows[i])
	}
	return out
}

func (m *tuiModel) menuView(w, room int) []string {
	items := menuItems(m.v, m.fresh && !m.pending)
	if m.cursor >= len(items) {
		m.cursor = 0
	}
	out := []string{styleBold.Render(" Actions available now"), ""}
	var rows []string
	for i, item := range items {
		row := "   " + item.Label
		if i == m.cursor {
			row = styleFocus.Render(fit(" ▸ "+item.Label+" ", w))
		}
		rows = append(rows, row)
	}
	hint := wrapIndent(items[m.cursor].Hint, w, "   ")
	out = append(out, m.list(rows, m.cursor, room-len(out)-len(hint)-1)...)
	out = append(out, "")
	for _, line := range hint {
		out = append(out, styleDim.Render(line))
	}
	if !m.fresh {
		out = append(out, styleFailed.Render(" Reconnecting: only views are listed until fresh state arrives."))
	}
	return out
}

func (m *tuiModel) setupView(w int) []string {
	out := []string{styleBold.Render(fmt.Sprintf(" Set up review · step %d of 4", m.setupStep+1)), ""}
	agent := ""
	if len(m.agents) > 0 {
		agent = m.agents[m.agentPick]
	}
	rows := []struct{ name, value string }{{"Agent", agent}, {"Intent", m.setupValues[1]}, {"Target", m.setupValues[2]}, {"Confirm", "save intent and target"}}
	for i, row := range rows {
		mark := "○"
		if i < m.setupStep {
			mark = styleDone.Render("✓")
		}
		if i == m.setupStep {
			mark = "▸"
		}
		value := row.value
		if i == m.setupStep && m.editing {
			value = m.input.View()
		}
		out = append(out, fit(fmt.Sprintf(" %s %-8s %s", mark, row.name, value), w))
		if i == 0 && m.setupStep == 0 {
			for j, name := range m.agents {
				line := "     " + agentLabel(name, "")
				if j == m.agentPick {
					line = styleFocus.Render(fit("   ▸ "+agentLabel(name, "")+" ", w))
				}
				out = append(out, line)
			}
		}
	}
	out = append(out, "")
	var help string
	switch m.setupStep {
	case 0:
		help = "Greenhouse uses the agent's own login, provider and default model. It never runs login or copies credentials."
	case 1:
		help = "Describe what the committed change should do. The reviewer checks the diff against this intent."
	case 2:
		help = "The review covers committed changes from this Git target to your head. Dirty files are excluded."
	case 3:
		help = "Enter saves the agent, intent and target. Nothing runs yet: loading rules and starting the review stay separate steps."
	}
	for _, line := range wrapIndent(help, w, " ") {
		out = append(out, styleDim.Render(line))
	}
	return out
}

func (m *tuiModel) findingsView(w, room int) []string {
	r := m.v.Run
	actionable, informational := outstanding(r)
	labels := map[string]string{}
	for _, f := range actionable {
		labels[f.ID] = "open"
	}
	for _, f := range informational {
		labels[f.ID] = "info"
	}
	kinds := resolvedKinds(r)
	canRepair := has(m.v.Actions, "repair")
	ids := selectable(r)
	selected := 0
	var rows []string
	for i, f := range r.Findings {
		state := labels[f.ID]
		if state == "" {
			state = "resolved " + kinds[f.ID]
		}
		box := ""
		if canRepair {
			box = "    "
			if has(ids, f.ID) {
				box = "[ ] "
				if m.selected[f.ID] {
					box = "[x] "
					selected++
				}
			}
		}
		row := fmt.Sprintf("%s%s %s:%d %s · %s", box, f.Priority, f.File, f.Start, state, f.Explanation)
		if i == m.cursor {
			rows = append(rows, styleFocus.Render(fit(" ▸ "+row+" ", w)))
		} else {
			rows = append(rows, fit("   "+row, w))
		}
	}
	title := " Findings · " + findingCount(len(actionable), len(informational))
	if canRepair {
		title += fmt.Sprintf(" · %d selected", selected)
	}
	out := []string{styleBold.Render(fit(title, w)), ""}
	detail := m.findingDetail(r.Findings[m.cursor], w-2)
	if w >= 99 {
		left := w * 45 / 100
		listRows := make([]string, len(r.Findings))
		for i, f := range r.Findings {
			marker := "   "
			if i == m.cursor {
				marker = " ▸ "
			}
			row := fit(marker+findingRow(f), left-1)
			if i == m.cursor {
				row = styleFocus.Render(row)
			}
			if canRepair && has(ids, f.ID) && m.selected[f.ID] {
				row = fit(row+" [x]", left-1)
			}
			listRows[i] = row
		}
		listRows = m.list(listRows, m.cursor, room-len(out))
		detail = m.findingDetail(r.Findings[m.cursor], w-left-2)
		for i := 0; i < max(len(listRows), len(detail)) && len(out) < room; i++ {
			l, d := "", ""
			if i < len(listRows) {
				l = listRows[i]
			}
			if i < len(detail) {
				d = detail[i]
			}
			out = append(out, l+strings.Repeat(" ", max(0, left-ansi.StringWidth(l)))+"│ "+d)
		}
		return out
	}
	if !m.expanded {
		out = append(out, m.list(rows, m.cursor, room-len(out)-1)...)
		return append(out, "", styleDim.Render("   Enter shows the full finding."))
	}
	listRoom := min(len(rows), max(2, (room-len(out))/3))
	out = append(out, m.list(rows, m.cursor, listRoom)...)
	out = append(out, styleDim.Render(" "+strings.Repeat("─", w-2)))
	return append(out, detail...)
}

func (m *tuiModel) findingDetail(f Finding, w int) []string {
	var out []string
	add := func(text string) { out = append(out, wrap(text, w)...) }
	location := fmt.Sprintf("%s:%d", f.File, f.Start)
	if f.End > f.Start {
		location += fmt.Sprintf("-%d", f.End)
	}
	add(styleBold.Render(f.Priority+" "+f.Origin) + " · " + location)
	add(f.Explanation)
	if f.Action != "" {
		add("Action: " + f.Action)
	}
	if f.Rule != "" {
		add("Rule: " + f.Rule)
	}
	if f.Head != "" {
		add("Reported for head " + targetLabel(f.Head))
	}
	for _, line := range responseLines(m.v.Run)[f.ID] {
		add(strings.TrimSpace(line))
	}
	return out
}

func (m *tuiModel) decisionView(w, room int) []string {
	d := m.v.Decision
	out := []string{styleBold.Render(fit(" Decision required: "+d.Kind+" at "+d.Stage, w)), ""}
	out = append(out, wrapIndent(d.Reason, w, " ")...)
	if d.UserOnly {
		out = append(out, wrapIndent("Only you can answer this ("+d.Category+"); auto mode never does.", w, " ")...)
	}
	out = append(out, "")
	switch m.decision.step {
	case 0:
		var rows []string
		for i, option := range d.Options {
			row := "   " + option
			if i == m.decision.choice {
				row = styleFocus.Render(" ▸ " + option + " ")
			}
			rows = append(rows, row)
		}
		out = append(out, " Choose an answer:")
		out = append(out, m.list(rows, m.decision.choice, room-len(out))...)
	case 1:
		out = append(out, " Findings to repair (all selected means the whole group):")
		var rows []string
		for i, id := range d.Findings {
			box := "[ ] "
			if m.selected[id] {
				box = "[x] "
			}
			row := box + m.findingLabel(id)
			if i == m.cursor {
				rows = append(rows, styleFocus.Render(fit(" ▸ "+row+" ", w)))
			} else {
				rows = append(rows, fit("   "+row, w))
			}
		}
		out = append(out, m.list(rows, m.cursor, room-len(out))...)
	case 2:
		out = append(out, " Answer: increase-cap", " New combined limit:", " "+m.input.View())
	case 3:
		out = append(out, fit(" Answer: "+m.decisionChoice(), w), " Reason (recorded with the decision):", " "+m.input.View())
	}
	return out
}

func (m *tuiModel) findingLabel(id string) string {
	for _, f := range m.v.Run.Findings {
		if f.ID == id {
			return findingRow(f)
		}
	}
	return displayID(id, 24)
}

func (m *tuiModel) recoveryView(w, room int) []string {
	r := m.v.Run
	var evidence strings.Builder
	renderRecovery(&evidence, r)
	out := []string{styleBold.Render(" Recovery required"), ""}
	for _, line := range strings.Split(strings.TrimRight(evidence.String(), "\n"), "\n") {
		if strings.HasPrefix(line, "[e]") || strings.HasPrefix(line, "Options:") {
			continue
		}
		out = append(out, wrapIndent(line, w, " ")...)
	}
	out = append(out, "")
	if m.recovery.step == 1 {
		out = append(out, fit(" Recovery: "+r.Recovery.Options[m.recovery.choice], w), " Reason (recorded with the recovery):", " "+m.input.View())
		return out
	}
	options := []string{" Choose a recovery:"}
	for i, option := range r.Recovery.Options {
		row := "   " + option
		if i == m.recovery.choice {
			row = styleFocus.Render(" ▸ " + option + " ")
		}
		options = append(options, row)
	}
	if len(out)+len(options) > room {
		out = out[:max(2, room-len(options))]
	}
	return append(out, options...)
}

func (m *tuiModel) settingsView(w, room int) []string {
	current := m.current()
	sources := fieldSources(m.v.Run.Configs[len(m.v.Run.Configs)-1])
	out := []string{styleBold.Render(" Settings draft") + styleDim.Render(" · changes apply only when you choose Apply or Save"), ""}
	var rows []string
	for i, name := range settingNames {
		value := fieldValue(*m.draft, i)
		if value == "" {
			value = "default"
		}
		if m.editing && i == m.cursor {
			value = m.input.View()
		}
		source := sources[settingKeys[i]]
		mark := " "
		if fieldValue(*m.draft, i) != fieldValue(current, i) {
			mark, source = "*", "draft, was "+orDefault(fieldValue(current, i))
		}
		row := fmt.Sprintf("%s %-15s %s", mark, name, value)
		if w >= 70 {
			row = fmt.Sprintf("%-40s %s", fit(row, 40), styleDim.Render(source))
		}
		rows = append(rows, row)
	}
	rows = append(rows, "")
	for _, t := range settingsTargets(m.v) {
		rows = append(rows, " "+t.label)
	}
	rows = append(rows, " Reset draft")
	cursorRow := m.cursor
	if m.cursor >= len(settingNames) {
		cursorRow++
	}
	for i := range rows {
		if i == cursorRow && !m.editing {
			rows[i] = styleFocus.Render(fit(" ▸"+ansi.Strip(rows[i])+" ", w))
		} else {
			rows[i] = fit("  "+rows[i], w)
		}
	}
	hint := wrapIndent(m.settingsHint(), w, " ")
	if len(hint) > 3 {
		hint = hint[:3]
	}
	out = append(out, m.list(rows, cursorRow, room-len(out)-len(hint)-1)...)
	out = append(out, "")
	for _, line := range hint {
		out = append(out, styleDim.Render(line))
	}
	return out
}

func orDefault(value string) string {
	if value == "" {
		return "default"
	}
	return value
}

func (m *tuiModel) settingsHint() string {
	targets := settingsTargets(m.v)
	switch {
	case m.cursor < len(settingNames):
		source := fieldSources(m.v.Run.Configs[len(m.v.Run.Configs)-1])[settingKeys[m.cursor]]
		switch m.cursor {
		case 4, 6:
			backend := m.draft.ReviewerBackend
			if m.cursor == 6 {
				backend = m.draft.FixerBackend
			}
			if backend == backendPi {
				models := strings.Join(m.models, ", ")
				if m.modelsErr != "" {
					models = "catalog unavailable: " + m.modelsErr
				} else if models == "" {
					models = "loading catalog"
				}
				return "Empty uses Pi's default. Pi models: " + models
			}
			return "Empty uses the agent's native default; or enter an exact model ID. Current source: " + source + "."
		case 7:
			return "Rule selection does not load rules; load them from the home screen after Apply. Current source: " + source + "."
		case 2:
			return "Combined review and repair rounds for a run. Current source: " + source + "."
		}
		return "Current source: " + source + "."
	case m.cursor < len(settingNames)+len(targets):
		return targets[m.cursor-len(settingNames)].scope
	}
	return "Discard the draft and start again from the run's current settings."
}

func (m *tuiModel) pageView(w, room int) []string {
	text := m.pageText
	if m.pageKind == "details" {
		var b strings.Builder
		b.WriteString(statePage(m.v, m.live, m.criteria))
		if m.criteria && m.v.Run.SelectedDiff != "" {
			b.WriteString("\nSelected committed diff:\n" + m.v.Run.SelectedDiff)
		}
		text = b.String()
	}
	lines := wrap(text, w-1)
	view := room - 2
	m.scroll = min(m.scroll, max(0, len(lines)-view))
	end := min(len(lines), m.scroll+view)
	position := ""
	if len(lines) > view {
		position = fmt.Sprintf(" · lines %d-%d of %d", m.scroll+1, end, len(lines))
	}
	out := []string{styleBold.Render(" "+m.pageTitle) + styleDim.Render(position), ""}
	for _, line := range lines[m.scroll:end] {
		out = append(out, " "+line)
	}
	return out
}

func (m *tuiModel) confirmView(w int) []string {
	a := m.confirm
	out := []string{styleBold.Render(fit(" "+a.Label, w)), ""}
	out = append(out, wrapIndent(a.Hint, w, " ")...)
	out = append(out, "")
	switch a.Confirm {
	case "x":
		out = append(out, wrapIndent("Press x again to cancel "+progressLabel(m.v.Run)+". Esc keeps it running. q only detaches.", w, " ")...)
	case "":
		out = append(out, " Press Enter to confirm or Esc to go back.")
	default:
		out = append(out, fit(" Type "+a.Confirm+" and press Enter to confirm:", w), " "+m.input.View())
	}
	return out
}

func (m *tuiModel) homeSteps() []tuiStep {
	steps := setupSteps(m.v, m.agents, m.suggestion)
	if !has(m.v.Actions, "prepare") || len(m.agents) == 0 && m.v.Run.Reviewer == nil {
		return steps
	}
	agent := m.planAgent()
	state := stepTodo
	if r := m.v.Run; r.Reviewer != nil && backendOf(*r.Reviewer) == agent {
		state = stepDone
	}
	steps[0] = tuiStep{"Agent", state, agentLabel(agent, "")}
	if agent == m.current().ReviewerBackend {
		steps[0].Text = agentLabel(agent, m.current().ReviewerModel)
	}
	steps[1].State, steps[1].Text = m.intentLabel()
	target := m.planTarget()
	state = stepTodo
	if m.plan.target == "" && m.v.Run.Target != "" {
		state = stepDone
	} else if m.plan.target == "" {
		target += " (suggested)"
	}
	steps[2] = tuiStep{"Target", state, targetLabel(strings.TrimSuffix(target, " (suggested)"))}
	if strings.HasSuffix(target, " (suggested)") {
		steps[2].Text += " (suggested)"
	}
	if m.planTarget() == "" {
		steps[2].Text = "no default branch found; choose a target"
	}
	return steps
}

func (m *tuiModel) intentView(w, room int) []string {
	out := []string{styleBold.Render(" Which intent should Run use?"), ""}
	out = append(out, wrapIndent(m.question, w, " ")...)
	out = append(out, "")
	if m.editing {
		out = append(out, " "+m.input.View(), "")
		for _, line := range wrapIndent("Enter continues Run; Esc stops it. Nothing was sent yet.", w, " ") {
			out = append(out, styleDim.Render(line))
		}
		return out
	}
	var rows []string
	for i, c := range m.candidates {
		row := c.Label() + ": " + c.FirstLine()
		if i == m.cursor {
			rows = append(rows, styleFocus.Render(fit(" ▸ "+row+" ", w)))
		} else {
			rows = append(rows, fit("   "+row, w))
		}
	}
	other := "Type the intent myself"
	if m.cursor == len(m.candidates) {
		rows = append(rows, styleFocus.Render(" ▸ "+other+" "))
	} else {
		rows = append(rows, "   "+other)
	}
	return append(out, m.list(rows, m.cursor, room-len(out))...)
}

func agentName(backend, model string) string {
	name := backendNames[backend]
	if name == "" {
		name = backendNames[backendPi]
	}
	if model != "" {
		name += " " + model
	}
	return name
}

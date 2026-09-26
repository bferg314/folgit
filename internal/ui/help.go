package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderHelp is the full key reference: the status bar keeps the screen
// clean, so everything lives here behind "?".
func (a *App) renderHelp() string {
	st := a.st
	const keyW, descW = 11, 36
	row := func(k, d string) string { return fit(st.key.Render(k), keyW) + fit(st.textS.Render(d), descW) }
	title := func(t string) string { return fit(st.boxTitle.Render(t), keyW+descW) }

	left := []string{
		title("Everywhere"),
		row("tab / 1-3", "switch tabs"),
		row("↑↓ / j k", "move"),
		row("pgup/pgdn", "page"),
		row("/", "fuzzy filter (esc clears)"),
		row("?", "this help (any key closes)"),
		row("q", "quit"),
		"",
		title("Local"),
		row("enter", "open in a tool…"),
		row("g", "quit and cd into the repo"),
		row("p / P", "pull this repo · all clean repos"),
		row("F", "fetch all repos"),
		row("d", "show / hide the detail pane"),
		row("J / K", "scroll details (ctrl+d/u: half page)"),
		row("w", "open the repo's web page"),
		row("i", "GitHub issues"),
		row("b / B", "stale branches: this repo · all"),
		row("s", "sort: recent · name · dirty"),
		row("r", "rescan"),
	}

	right := []string{title("Tools")}
	tools := a.enabledTools()
	for _, t := range tools {
		k := t.Key
		if reservedKeys[k] {
			k = "(" + k + ")"
		}
		right = append(right, row(k, t.Name))
	}
	if len(tools) == 0 {
		right = append(right, fit(st.dim.Render("none enabled: see Settings"), keyW+descW))
	}
	right = append(right,
		"",
		title("Remote"),
		row("space / a", "select · select all"),
		row("enter", "clone selected (or current)"),
		row("w", "open the repo's web page"),
		row("i", "GitHub issues"),
		row("R", "reload from GitHub"),
		"",
		title("Settings"),
		row("space", "toggle"),
		row("enter / x", "default folder: set to current · clear"),
	)

	legend := func(sym, text string) string { return fit(sym, 5) + fit(st.textS.Render(text), 18) }
	status := []string{
		title("Status"),
		legend(st.ok.Render("✓"), "clean, in sync") + legend(st.warn.Render("●3"), "changed files") +
			legend(st.bad.Render("!2"), "conflicts") + legend(st.dim.Render("≡1"), "stashes"),
		legend(st.ahead.Render("↑2"), "to push") + legend(st.behind.Render("↓5"), "to pull") +
			legend(st.dim.Render("∅"), "no upstream"),
	}

	var body string
	if a.w >= 2*(keyW+descW)+12 {
		for len(right) < len(left) {
			right = append(right, "")
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(left, "\n"), "    ", strings.Join(right, "\n"))
	} else {
		body = strings.Join(append(append(left, ""), right...), "\n")
		status = []string{title("Status"),
			legend(st.ok.Render("✓"), "clean, in sync") + legend(st.warn.Render("●3"), "changed files"),
			legend(st.bad.Render("!2"), "conflicts") + legend(st.dim.Render("≡1"), "stashes"),
			legend(st.ahead.Render("↑2"), "to push") + legend(st.behind.Render("↓5"), "to pull"),
			legend(st.dim.Render("∅"), "no upstream"),
		}
	}
	footer := st.dim.Render("Tools, keys and scan options: ") + st.textS.Render(tildePath(a.cfgPath))
	return st.box.Padding(0, 2).Render(body + "\n\n" + strings.Join(status, "\n") + "\n\n" + footer)
}

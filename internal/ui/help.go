package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Help geometry: a key column and a description column.
const helpKeyW, helpDescW = 11, 36

// helpLines is the key reference for what's on screen: the open popup, or
// the current tab plus the keys that work everywhere. Showing only these
// keeps it short enough to fit most terminals; renderHelp scrolls the rest.
func (a *App) helpLines() []string {
	st := a.st
	row := func(k, d string) string { return fit(st.key.Render(k), helpKeyW) + fit(st.textS.Render(d), helpDescW) }
	title := func(t string) string { return fit(st.boxTitle.Render(t), helpKeyW+helpDescW) }
	note := func(t string) string { return st.dim.Render(t) }

	switch {
	case a.cleanup != nil:
		return []string{
			title("Branch cleanup"),
			row("↑↓ / j k", "move"),
			row("space / a", "select · select all"),
			row("enter / d", "delete selected (y confirms)"),
			row("f", "fetch --prune and check again"),
			row("esc", "close"),
			"",
			note("These keys work while the branch list is open."),
		}
	case a.issues != nil:
		return []string{
			title("Issues"),
			row("↑↓ / j k", "move"),
			row("enter / o", "open the issue in your browser"),
			row("n", "new issue"),
			row("r", "refresh"),
			row("esc", "close"),
			"",
			note("These keys work while the issue list is open."),
		}
	case a.switcher != nil:
		return []string{
			title("Switch branch"),
			row("↑↓ / j k", "move"),
			row("enter", "switch to the highlighted branch"),
			row("/", "filter (enter switches to the top)"),
			row("f", "fetch, to list new remote branches"),
			row("esc", "close"),
			"",
			note("● is the current branch. Remote branches get a"),
			note("local branch that tracks them when you switch."),
		}
	case a.checks != nil:
		return []string{
			title("Repo checks"),
			row("↑↓ / j k", "move (shows what the fix does)"),
			row("enter", "fix the highlighted check"),
			row("esc", "close"),
			"",
			note("Line-ending fixes need no uncommitted changes"),
			note("and leave their changes for you to commit."),
		}
	case a.popup != nil:
		return []string{
			title("Open in…"),
			row("↑↓ / j k", "move"),
			row("enter", "open in the highlighted tool"),
			row("tool key", "open in that tool directly"),
			row("esc", "close"),
			"",
			note("These keys work while this popup is open."),
		}
	}

	everywhere := []string{
		title("Everywhere"),
		row("tab / 1-3", "switch tabs"),
		row("↑↓ / j k", "move"),
	}
	if a.tab != tabSettings {
		everywhere = append(everywhere,
			row("pgup/pgdn", "page"),
			row("/", "fuzzy filter (esc clears)"),
		)
	}
	everywhere = append(everywhere,
		row("?", "this help"),
		row("q", "quit"),
	)
	if a.cfg.Mouse {
		everywhere = append(everywhere,
			row("click", "go there; tab names switch tabs"),
			row("dbl-click", "enter · right-click: open in…"),
			row("wheel", "move, or scroll the detail pane"),
		)
	}

	var lines []string
	switch a.tab {
	case tabLocal:
		lines = a.localHelp(everywhere, row, title)
	case tabRemote:
		lines = append(everywhere, "",
			title("Remote"),
			row("space / a", "select · select all"),
			row("enter", "clone selected (or current)"),
			row("w", "open the repo's web page"),
			row("i", "GitHub issues"),
			row("m", "only my repos · everyone's"),
			row("R", "reload from GitHub"),
		)
	case tabSettings:
		lines = append(everywhere, "",
			title("Settings"),
			row("space", "toggle"),
			row("enter / x", "default folder: use current · clear"),
			row("enter / x", "clone protocol: change · follow gh"),
		)
	}
	lines = append(lines, "", note("Keys for the "+tabNames[a.tab]+" tab. Other tabs have their own."))
	if a.tab == tabSettings {
		lines = append(lines, note("Tools, keys and scan options: ")+st.textS.Render(tildePath(a.cfgPath)))
	}
	return lines
}

// localHelp is the Local tab's keys, its tools and the status legend. On
// wide terminals it is two balanced columns: everywhere keys and tools on
// the left, Local keys and the status legend on the right.
func (a *App) localHelp(everywhere []string, row func(k, d string) string, title func(t string) string) []string {
	st := a.st
	local := []string{
		title("Local"),
		row("enter", "open in a tool…"),
		row("g", "quit and cd into the repo"),
		row("p / P / F", "pull · pull all clean · fetch all"),
		row("d", "show / hide the detail pane"),
		row("J / K", "scroll details (ctrl+d/u: half page)"),
		row("w", "open the repo's web page"),
		row("i", "GitHub issues (n: new issue)"),
		row("S / C", "switch branch · repo checks"),
		row("b / B", "stale branches: this repo · all"),
		row("s", "sort: recent · name · dirty"),
		row("* / h / H", "pin to top · hide · show hidden"),
		row("D", "delete the repo's folder (to trash)"),
		row("r", "rescan"),
	}

	tools := []string{title("Tools")}
	enabled := a.enabledTools()
	for _, t := range enabled {
		k := t.Key
		if reservedKeys[k] {
			k = "(" + k + ")"
		}
		tools = append(tools, row(k, t.Name))
	}
	if len(enabled) == 0 {
		tools = append(tools, fit(st.dim.Render("none enabled: see Settings"), helpKeyW+helpDescW))
	}

	type sym struct{ s, text string }
	legend := []sym{
		{st.ok.Render("✓"), "clean, in sync"}, {st.warn.Render("●3"), "changed files"},
		{st.bad.Render("!2"), "conflicts"}, {st.dim.Render("≡1"), "stashes"},
		{st.ahead.Render("↑2"), "to push"}, {st.behind.Render("↓5"), "to pull"},
		{st.dim.Render("∅"), "no upstream"}, {st.warn.Render("⚑"), "a check failed: C"},
	}
	status := []string{"", fit(st.boxTitle.Render("Status"), helpKeyW+helpDescW)}
	for i := 0; i < len(legend); i += 2 {
		line := fit(legend[i].s, 5) + fit(st.textS.Render(legend[i].text), 18)
		if i+1 < len(legend) {
			line += fit(legend[i+1].s, 5) + fit(st.textS.Render(legend[i+1].text), 18)
		}
		status = append(status, line)
	}
	if a.w < 2*(helpKeyW+helpDescW)+12 {
		lines := append(append(append(everywhere, ""), local...), "")
		return append(append(lines, tools...), status...)
	}
	left := append(append(everywhere, ""), tools...)
	right := append(local, status...)
	for len(right) < len(left) {
		right = append(right, "")
	}
	for len(left) < len(right) {
		left = append(left, "")
	}
	cols := lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(left, "\n"), "    ", strings.Join(right, "\n"))
	return strings.Split(cols, "\n")
}

// helpRoom is how many help lines fit inside the box's border.
func (a *App) helpRoom() int { return max(3, a.h-2) }

// helpScrolls reports whether the help is taller than the terminal.
func (a *App) helpScrolls() bool { return len(a.helpLines()) > a.helpRoom() }

// renderHelp boxes the help, scrolled to a.helpOffset when it doesn't fit.
func (a *App) renderHelp() string {
	st := a.st
	lines := a.helpLines()
	if room := a.helpRoom(); len(lines) > room {
		show := room - 1 // the last line says how to scroll
		a.helpOffset = max(0, min(a.helpOffset, len(lines)-show))
		end := a.helpOffset + show
		more := st.key.Render("j k") + st.dim.Render(fmt.Sprintf(" scroll · lines %d–%d of %d · any other key closes", a.helpOffset+1, end, len(lines)))
		lines = append(lines[a.helpOffset:end:end], more)
	}
	return a.wheelOver(st.box.Padding(0, 2).Render(strings.Join(lines, "\n")), func(d int) tea.Cmd {
		a.helpOffset += d // renderHelp keeps it in range
		return nil
	})
}

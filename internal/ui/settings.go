package ui

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/launcher"
)

type settingsTab struct {
	cursor int
}

type settingItem struct {
	section string
	label   string
	detail  string
	on      *bool
	warn    string
	remote  bool         // changing it reloads the remote list
	tool    *config.Tool // set for tool rows

	// Action rows have no checkbox (on == nil): enter/space runs set, x
	// runs clear.
	set, clear func() tea.Cmd
}

func (a *App) settingItems() []settingItem {
	zellij := settingItem{section: "General", label: "Zellij tabs",
		detail: "inside zellij, terminal tools open in a new tab", on: &a.cfg.ZellijTabs}
	if !launcher.InZellij() {
		zellij.detail += " · not in zellij now"
	}
	items := []settingItem{a.defaultDirItem(), zellij}
	for i := range a.cfg.Tools {
		t := &a.cfg.Tools[i]
		it := settingItem{
			section: "Tools",
			label:   t.Name,
			detail:  "[" + t.Key + "]  " + strings.TrimSpace(t.Cmd+" "+strings.Join(t.Args, " ")) + "  · " + t.Mode,
			on:      &t.Enabled,
			tool:    t,
		}
		switch {
		case !config.Available(t.Cmd):
			it.warn = "not installed"
		case reservedKeys[t.Key]:
			it.warn = "key " + t.Key + " is reserved"
		}
		items = append(items, it)
	}
	gh := &a.cfg.GitHub
	items = append(items,
		settingItem{section: "GitHub", label: "Organization repos", on: &gh.IncludeOrgs, remote: true},
		settingItem{section: "GitHub", label: "Starred repos", on: &gh.IncludeStarred, remote: true},
		settingItem{section: "GitHub", label: "Forks", on: &gh.IncludeForks, remote: true},
		settingItem{section: "GitHub", label: "Archived repos", on: &gh.IncludeArchived, remote: true},
		settingItem{section: "Scanning", label: "Scan hidden directories", on: &a.cfg.ScanHidden},
		settingItem{section: "Appearance", label: "Powerline status bar", detail: "arrow separators; needs a Nerd Font", on: &a.cfg.Powerline},
	)
	return items
}

// defaultDirItem shows default_dir and sets it to the folder being viewed.
func (a *App) defaultDirItem() settingItem {
	here := tildePath(a.root)
	it := settingItem{section: "General", label: "Default folder"}
	switch d := a.cfg.DefaultDir; {
	case d == "":
		it.detail = "not set: opens where you launch folgit · enter: use " + here
	case d == here:
		it.detail = d + " · x: clear"
	default:
		it.detail = d + " · enter: use " + here + " · x: clear"
		if fi, err := os.Stat(config.ExpandPath(d)); err != nil || !fi.IsDir() {
			it.warn = "folder not found"
		}
	}
	it.set = func() tea.Cmd {
		if a.cfg.DefaultDir == here {
			return nil
		}
		a.cfg.DefaultDir = here
		return tea.Batch(a.saveConfig(), a.notify(1, "folgit will open %s by default", here))
	}
	it.clear = func() tea.Cmd {
		if a.cfg.DefaultDir == "" {
			return nil
		}
		a.cfg.DefaultDir = ""
		return tea.Batch(a.saveConfig(), a.notify(1, "Default folder cleared: folgit opens where you launch it"))
	}
	return it
}

// setTab switches tabs. Opening Settings re-checks for tools installed
// while folgit was running.
func (a *App) setTab(tab int) tea.Cmd {
	a.tab = tab
	if tab != tabSettings {
		return nil
	}
	names := a.cfg.EnableInstalled()
	if len(names) == 0 {
		return nil
	}
	return tea.Batch(a.saveConfig(), a.notify(1, "Found and enabled %s", strings.Join(names, ", ")))
}

func (a *App) settingsKey(key string) tea.Cmd {
	items := a.settingItems()
	t := &a.settings
	switch key {
	case "up", "k":
		t.cursor = max(0, t.cursor-1)
	case "down", "j":
		t.cursor = min(len(items)-1, t.cursor+1)
	case "x":
		if it := items[t.cursor]; it.clear != nil {
			return it.clear()
		}
	case "space", "enter":
		it := items[t.cursor]
		if it.on == nil {
			if it.set != nil {
				return it.set()
			}
			return nil
		}
		*it.on = !*it.on
		if it.tool != nil {
			it.tool.AutoDisabled = false // a hand-made choice sticks
		}
		cmds := []tea.Cmd{a.saveConfig()}
		if it.remote {
			cmds = append(cmds, a.loadRemote())
		}
		if it.section == "Scanning" {
			cmds = append(cmds, a.startScan())
		}
		return tea.Batch(cmds...)
	}
	return nil
}

func (a *App) renderSettings(h int) string {
	st := a.st
	items := a.settingItems()
	var lines []string
	cursorLine := 0
	section := ""
	for i, it := range items {
		if it.section != section {
			section = it.section
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "  "+st.boxTitle.Render(section))
		}
		marker := "  "
		label := st.textS.Render(it.label)
		if i == a.settings.cursor {
			cursorLine = len(lines)
			marker = st.marker.Render("▌") + " "
			label = st.selName.Render(it.label)
		}
		var box string
		switch {
		case it.on == nil:
			box = st.key.Render(" ▸ ")
		case *it.on:
			box = st.ok.Render("[✓]")
		default:
			box = st.faintText.Render("[ ]")
		}
		line := marker + "  " + box + " " + fit(label, 26)
		switch {
		case it.warn != "" && it.on == nil:
			// Action rows show long paths; keep the warning where it can't be cut off.
			line += st.warn.Render(it.warn) + "  " + st.dim.Render(it.detail)
		case it.warn != "":
			line += st.dim.Render(it.detail) + "  " + st.warn.Render(it.warn)
		default:
			line += st.dim.Render(it.detail)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "",
		"    "+st.dim.Render("Add tools, change keys, the clone layout and ignored folders in the config file below."))

	// Scroll so the cursor (and its section heading) stay visible.
	if len(lines) > h {
		start := max(0, min(cursorLine-h/2, len(lines)-h))
		lines = lines[start : start+h]
	}
	return strings.Join(lines, "\n")
}

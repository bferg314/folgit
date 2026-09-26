package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/config"
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
}

func (a *App) settingItems() []settingItem {
	var items []settingItem
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
	)
	return items
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
	case "space", "enter":
		it := items[t.cursor]
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
		box := st.faintText.Render("[ ]")
		if *it.on {
			box = st.ok.Render("[✓]")
		}
		line := marker + "  " + box + " " + fit(label, 26) + st.dim.Render(it.detail)
		if it.warn != "" {
			line += "  " + st.warn.Render(it.warn)
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

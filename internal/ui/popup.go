package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/config"
)

// toolPopup lets the user pick which tool to open a repo in.
type toolPopup struct {
	repo   *localRepo
	tools  []config.Tool
	cursor int
}

func (a *App) popupKey(key string) tea.Cmd {
	p := a.popup
	switch key {
	case "esc", "q":
		a.popup = nil
	case "up", "k":
		p.cursor = (p.cursor + len(p.tools) - 1) % len(p.tools)
	case "down", "j":
		p.cursor = (p.cursor + 1) % len(p.tools)
	case "enter":
		a.popup = nil
		return a.launch(p.tools[p.cursor], p.repo.path)
	default:
		for _, t := range p.tools {
			if t.Key == key {
				a.popup = nil
				return a.launch(t, p.repo.path)
			}
		}
	}
	return nil
}

func (a *App) renderPopup() string {
	st := a.st
	p := a.popup
	lines := []string{st.dim.Render("Open ") + st.bold.Render(p.repo.rel) + st.dim.Render(" in…"), ""}
	for i, t := range p.tools {
		marker := "  "
		name := st.textS.Render(t.Name)
		if i == p.cursor {
			marker = st.marker.Render("▌ ")
			name = st.selName.Render(t.Name)
		}
		mode := ""
		if t.Mode == config.ModeTerminal {
			mode = st.faintText.Render(" terminal")
		}
		lines = append(lines, marker+st.key.Render(t.Key)+"  "+fit(name, 16)+mode)
	}
	return st.box.Render(strings.Join(lines, "\n"))
}

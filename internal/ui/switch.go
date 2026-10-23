package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/launcher"
)

// switchView is the popup for switching the selected repo's branch.
type switchView struct {
	path, rel string
	loading   bool
	fetching  bool
	switching bool
	all       []gitinfo.SwitchTarget
	view      []gitinfo.SwitchTarget
	matches   map[string][]int
	filter    textinput.Model
	err       error // listing or switching failed
	cursor    int
	offset    int
}

type (
	switchListMsg struct {
		view     *switchView
		branches []gitinfo.SwitchTarget
		err      error
	}
	switchDoneMsg struct {
		view   *switchView
		branch string
		err    error
	}
)

// openSwitch lists the selected repo's branches.
func (a *App) openSwitch() tea.Cmd {
	r := a.local.selected()
	if r == nil {
		return nil
	}
	if r.busy != "" {
		return a.notify(2, "%s is busy %s", r.rel, r.busy)
	}
	a.switcher = &switchView{path: r.path, rel: r.rel, filter: newFilter()}
	return a.loadSwitch(false)
}

// loadSwitch (re)lists branches, optionally fetching first so new remote
// branches show up.
func (a *App) loadSwitch(fetch bool) tea.Cmd {
	v := a.switcher
	v.loading, v.fetching, v.err = true, fetch, nil
	path := v.path
	return tea.Batch(a.startSpinner(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if fetch {
			if err := launcher.Fetch(ctx, path); err != nil {
				return switchListMsg{view: v, err: fmt.Errorf("fetch failed: %w", err)}
			}
		}
		bs, err := gitinfo.SwitchTargets(ctx, path)
		return switchListMsg{view: v, branches: bs, err: err}
	})
}

func (a *App) handleSwitchList(msg switchListMsg) {
	v := a.switcher
	if v == nil || v != msg.view {
		return
	}
	v.loading, v.fetching, v.err = false, false, msg.err
	if msg.err == nil {
		v.all = msg.branches
	}
	v.refresh()
}

// refresh applies the filter, keeping the cursor on the same branch.
func (v *switchView) refresh() {
	var keep string
	if t := v.current(); t != nil {
		keep = t.Name
	}
	v.view, v.matches = v.all, nil
	if q := v.filter.Value(); q != "" {
		names := make([]string, len(v.all))
		for i, t := range v.all {
			names[i] = t.Name
		}
		ms := fuzzy.Find(q, names)
		v.view = make([]gitinfo.SwitchTarget, len(ms))
		v.matches = make(map[string][]int, len(ms))
		for i, m := range ms {
			v.view[i] = v.all[m.Index]
			v.matches[v.view[i].Name] = m.MatchedIndexes
		}
	}
	v.cursor = min(v.cursor, max(0, len(v.view)-1))
	for i, t := range v.view {
		if t.Name == keep {
			v.cursor = i
			break
		}
	}
}

func (v *switchView) current() *gitinfo.SwitchTarget {
	if v.cursor >= 0 && v.cursor < len(v.view) {
		return &v.view[v.cursor]
	}
	return nil
}

// switchTo checks out the highlighted branch.
func (a *App) switchTo() tea.Cmd {
	v := a.switcher
	t := v.current()
	if t == nil || v.loading || v.switching {
		return nil
	}
	if t.Current {
		a.switcher = nil
		return a.notify(1, "%s is already on %s", v.rel, t.Name)
	}
	v.switching, v.err = true, nil
	path, name, track := v.path, t.Name, t.Remote
	local := t.LocalName()
	return tea.Batch(a.startSpinner(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return switchDoneMsg{view: v, branch: local, err: launcher.Switch(ctx, path, name, track)}
	})
}

func (a *App) handleSwitchDone(msg switchDoneMsg) tea.Cmd {
	v := msg.view
	v.switching = false
	if msg.err != nil {
		// Leave the popup open so the reason can be read. git's own message
		// for the usual one ends in a file list that doesn't fit here.
		v.err = msg.err
		if strings.Contains(msg.err.Error(), "would be overwritten") {
			v.err = errors.New("your local changes would be overwritten: commit or stash them first")
		}
		return nil
	}
	if a.switcher == v {
		a.switcher = nil
	}
	a.invalidateDetails(v.path)
	return tea.Batch(refreshStatus(v.path), a.notify(1, "Switched %s to %s", v.rel, msg.branch))
}

func (a *App) switchKey(msg tea.KeyPressMsg) tea.Cmd {
	v := a.switcher
	key := msg.String()
	if v.switching {
		return nil
	}
	if v.filter.Focused() {
		switch key {
		case "esc":
			v.filter.SetValue("")
			v.filter.Blur()
		case "enter":
			v.filter.Blur()
			return a.switchTo()
		case "up", "down":
			v.filter.Blur()
		default:
			var cmd tea.Cmd
			v.filter, cmd = v.filter.Update(msg)
			v.refresh()
			v.cursor = 0 // the best match
			return cmd
		}
		v.refresh()
		return nil
	}

	page := max(1, a.popupListHeight())
	move := func(d int) { v.cursor = max(0, min(len(v.view)-1, v.cursor+d)) }
	switch key {
	case "esc":
		if v.filter.Value() != "" {
			v.filter.SetValue("")
			v.refresh()
			return nil
		}
		a.switcher = nil
	case "q", "S":
		a.switcher = nil
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "pgup":
		move(-page)
	case "pgdown":
		move(page)
	case "home":
		v.cursor = 0
	case "end":
		move(len(v.view))
	case "/":
		return v.filter.Focus()
	case "f":
		if !v.loading {
			return a.loadSwitch(true)
		}
	case "enter":
		return a.switchTo()
	}
	return nil
}

func (a *App) renderSwitch() string {
	st := a.st
	v := a.switcher
	bw, _ := a.popupBoxSize()
	cw := bw - 2 - 4
	listH := a.popupListHeight()

	title := st.boxTitle.Render("Switch branch") + st.dim.Render(" · ") + st.bold.Render(v.rel)
	var right string
	switch {
	case v.loading || v.switching:
		right = a.spin.View()
	default:
		right = st.dim.Render(fmt.Sprintf("%d branches", len(v.view)))
	}
	gap := max(1, cw-lipgloss.Width(title)-lipgloss.Width(right))
	lines := []string{fit(title+strings.Repeat(" ", gap)+right, cw)}
	if v.filter.Focused() || v.filter.Value() != "" {
		lines = append(lines, st.key.Render("/")+" "+v.filter.View())
	} else {
		lines = append(lines, "")
	}

	rowsH := listH
	if v.err != nil {
		rowsH = max(1, listH-2)
	}
	var body []string
	switch {
	case v.loading && v.fetching:
		body = []string{a.spin.View() + st.dim.Render(" Fetching…")}
	case v.loading && len(v.all) == 0:
		body = []string{a.spin.View() + st.dim.Render(" Loading branches…")}
	case len(v.view) == 0 && v.filter.Value() != "":
		body = []string{st.dim.Render("No branch matches that filter.")}
	case len(v.view) == 0 && v.err == nil:
		body = []string{st.dim.Render("No branches yet.")}
	default:
		body = a.switchRows(cw, rowsH)
	}
	if v.err != nil {
		for len(body) < rowsH {
			body = append(body, "")
		}
		body = append(body[:rowsH], "", st.bad.Render("✗ ")+st.dim.Render(v.err.Error()))
	}
	for len(body) < listH {
		body = append(body, "")
	}
	lines = append(lines, body[:listH]...)

	hint := st.key.Render("enter") + st.dim.Render(" switch · ") + st.key.Render("/") + st.dim.Render(" filter · ") +
		st.key.Render("f") + st.dim.Render(" fetch · ") + st.key.Render("esc") + st.dim.Render(" close")
	lines = append(lines, "", fit(hint, cw))
	a.boxZone(1, 0, zone{w: cw, click: func() tea.Cmd {
		if v.switching {
			return nil
		}
		return v.filter.Focus()
	}})
	return a.wheelOver(st.box.Width(bw).Render(strings.Join(lines, "\n")), func(d int) tea.Cmd {
		return a.switchKey(wheelKey(d))
	})
}

func (a *App) switchRows(cw, h int) []string {
	st := a.st
	v := a.switcher
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+h {
		v.offset = v.cursor - h + 1
	}
	v.offset = max(0, min(v.offset, len(v.view)-h))

	const agoW = 8
	nameW := 10
	for _, t := range v.view {
		nameW = max(nameW, len([]rune(t.Name)))
	}
	nameW = min(nameW, cw/2)
	infoW := max(10, cw-2-2-nameW-2-agoW-2)

	var rows []string
	for i := v.offset; i < min(len(v.view), v.offset+h); i++ {
		t := v.view[i]
		sel := i == v.cursor
		marker := "  "
		if sel {
			marker = st.marker.Render("▌") + " "
		}
		mark := "  "
		if t.Current {
			mark = st.ok.Render("● ")
		}

		var info string
		switch {
		case t.Current:
			info = st.ok.Render("current")
		case t.Remote:
			info = st.dim.Render("remote · creates local " + t.LocalName())
		case t.Track == "gone":
			info = st.warn.Render("remote deleted")
		case t.Upstream != "":
			info = st.dim.Render(t.Upstream)
			if t.Track != "" {
				info += " " + st.textS.Render(t.Track)
			}
		default:
			info = st.dim.Render("local only")
		}
		if t.Current && t.Track != "" {
			info += st.dim.Render(" · ") + st.textS.Render(t.Track)
		}

		name := st.highlightName(t.Name, nameW, v.matches[t.Name], sel)
		pick := func() bool {
			if v.switching {
				return false
			}
			was := v.cursor == i
			v.filter.Blur()
			v.cursor = i
			return was
		}
		a.boxRow(2+len(rows), cw, pick, a.switchTo, nil)
		rows = append(rows, marker+mark+name+"  "+fit(info, infoW)+"  "+
			fit(st.agoStyle(t.When).Render(ago(t.When)), agoW))
	}
	return rows
}

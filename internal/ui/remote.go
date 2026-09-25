package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sahilm/fuzzy"

	"github.com/bferg314/folgit/internal/launcher"
	"github.com/bferg314/folgit/internal/provider"
	"github.com/bferg314/folgit/internal/provider/github"
)

// At most this many clones run at once.
var cloneSlots = make(chan struct{}, 4)

type remoteTab struct {
	all      []provider.Repo
	view     []provider.Repo
	matches  map[string][]int
	missing  int // repos not present locally, before text filtering
	loading  bool
	err      error
	cursor   int
	offset   int
	filter   textinput.Model
	selected map[string]bool
	cloning  map[string]bool
	cloned   map[string]bool
	failed   map[string]string
}

func newRemoteTab() remoteTab {
	return remoteTab{
		filter:   newFilter(),
		selected: make(map[string]bool),
		cloning:  make(map[string]bool),
		cloned:   make(map[string]bool),
		failed:   make(map[string]string),
	}
}

func (t *remoteTab) current() *provider.Repo {
	if t.cursor >= 0 && t.cursor < len(t.view) {
		return &t.view[t.cursor]
	}
	return nil
}

// refresh shows repos that are not already cloned locally.
func (t *remoteTab) refresh(local map[string]bool) {
	var keep string
	if r := t.current(); r != nil {
		keep = r.Key()
	}

	var missing []provider.Repo
	for _, r := range t.all {
		if !local[r.Key()] && !t.cloned[r.Key()] {
			missing = append(missing, r)
		}
	}
	sort.SliceStable(missing, func(i, j int) bool { return missing[i].PushedAt.After(missing[j].PushedAt) })
	t.missing = len(missing)

	t.matches = nil
	if q := t.filter.Value(); q != "" {
		names := make([]string, len(missing))
		for i, r := range missing {
			names[i] = r.FullName()
		}
		ms := fuzzy.Find(q, names)
		t.matches = make(map[string][]int, len(ms))
		view := make([]provider.Repo, len(ms))
		for i, m := range ms {
			view[i] = missing[m.Index]
			t.matches[view[i].Key()] = m.MatchedIndexes
		}
		missing = view
	}
	t.view = missing

	t.cursor = min(t.cursor, max(0, len(t.view)-1))
	for i, r := range t.view {
		if r.Key() == keep {
			t.cursor = i
			break
		}
	}
}

// ---- keys ----

func (a *App) remoteKey(key string) tea.Cmd {
	t := &a.remote
	page := max(1, a.h-8)
	move := func(d int) { t.cursor = max(0, min(len(t.view)-1, t.cursor+d)) }
	switch key {
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "pgup":
		move(-page)
	case "pgdown":
		move(page)
	case "home":
		t.cursor = 0
	case "end":
		t.cursor = max(0, len(t.view)-1)
	case "space":
		if r := t.current(); r != nil {
			t.selected[r.Key()] = !t.selected[r.Key()]
			move(1)
		}
	case "a":
		all := true
		for _, r := range t.view {
			all = all && t.selected[r.Key()]
		}
		for _, r := range t.view {
			t.selected[r.Key()] = !all
		}
	case "R":
		return a.loadRemote()
	case "enter":
		var targets []provider.Repo
		for _, r := range t.view {
			if t.selected[r.Key()] {
				targets = append(targets, r)
			}
		}
		if len(targets) == 0 {
			if r := t.current(); r != nil {
				targets = append(targets, *r)
			}
		}
		var cmds []tea.Cmd
		for _, r := range targets {
			if t.cloning[r.Key()] {
				continue
			}
			delete(t.selected, r.Key())
			delete(t.failed, r.Key())
			t.cloning[r.Key()] = true
			cmds = append(cmds, a.clone(r))
		}
		if len(cmds) > 0 {
			cmds = append(cmds, a.startSpinner(), a.notify(0, "Cloning %d repo(s) into %s", len(cmds), a.root))
		}
		return tea.Batch(cmds...)
	}
	return nil
}

func (a *App) clone(r provider.Repo) tea.Cmd {
	dest := launcher.CloneDest(a.root, a.cfg.CloneLayout, r)
	return func() tea.Msg {
		cloneSlots <- struct{}{}
		defer func() { <-cloneSlots }()
		return cloneMsg{repo: r, dest: dest, err: launcher.Clone(context.Background(), r.CloneURL, dest)}
	}
}

// ---- view ----

func (a *App) renderRemote(h int) string {
	t := &a.remote
	st := a.st
	var lines []string

	if t.filter.Focused() || t.filter.Value() != "" {
		lines = append(lines, " "+st.key.Render("/")+" "+t.filter.View())
		h--
	}

	if len(t.view) == 0 {
		var msg string
		switch {
		case t.loading:
			msg = a.spin.View() + st.dim.Render(" Fetching your GitHub repositories…")
		case errors.Is(t.err, github.ErrNoToken):
			msg = st.warn.Render("Not signed in. ") + st.dim.Render("Run ") + st.key.Render("gh auth login") +
				st.dim.Render(" or set ") + st.key.Render("GITHUB_TOKEN") + st.dim.Render(", then press ") + st.key.Render("R")
		case t.err != nil:
			msg = st.bad.Render("Couldn't load repositories: ") + st.dim.Render(t.err.Error())
		case t.filter.Value() != "":
			msg = st.dim.Render("Nothing matches that filter.")
		default:
			msg = st.ok.Render("✓ ") + st.dim.Render("Every repository you have access to is already here.")
		}
		lines = append(lines, "", "   "+msg)
		return strings.Join(lines, "\n")
	}

	const langW, starW, agoW = 12, 6, 9
	nameW := 12
	for _, r := range t.view {
		nameW = max(nameW, len(r.FullName()))
	}
	nameW = min(nameW, 40)
	descW := a.w - 4 - nameW - langW - starW - agoW - 8
	showDesc := descW >= 12
	if !showDesc {
		nameW = max(10, a.w-4-langW-starW-agoW-6)
	}

	head := "    " + fit(st.colHead.Render("REPOSITORY"), nameW) + "  "
	if showDesc {
		head += fit(st.colHead.Render("DESCRIPTION"), descW) + "  "
	}
	head += fit(st.colHead.Render("LANGUAGE"), langW) + "  " + fit(st.colHead.Render("   ★"), starW) + "  " + fit(st.colHead.Render("PUSHED"), agoW)
	lines = append(lines, head)
	h--

	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+h {
		t.offset = t.cursor - h + 1
	}
	t.offset = max(0, min(t.offset, len(t.view)-h))

	for i := t.offset; i < min(len(t.view), t.offset+h); i++ {
		r := t.view[i]
		key := r.Key()
		sel := i == t.cursor

		marker := "  "
		if sel {
			marker = st.marker.Render("▌") + " "
		}
		check := st.faintText.Render("○ ")
		switch {
		case t.cloning[key]:
			check = a.spin.View() + " "
		case t.failed[key] != "":
			check = st.bad.Render("✗ ")
		case t.selected[key]:
			check = st.key.Render("◉ ")
		}

		line := marker + check + st.highlightName(r.FullName(), nameW, t.matches[key], sel) + "  "
		if showDesc {
			var badges []string
			if r.Archived {
				badges = append(badges, st.warn.Render("archived"))
			}
			if r.Fork {
				badges = append(badges, st.dim.Render("fork"))
			}
			if !r.Private {
				badges = append(badges, st.ok.Render("public"))
			}
			desc := strings.Join(badges, " ")
			if r.Description != "" {
				if desc != "" {
					desc += " "
				}
				desc += st.dim.Render(r.Description)
			}
			line += fit(desc, descW) + "  "
		}
		stars := ""
		if r.Stars > 0 {
			stars = fmt.Sprintf("%6d", r.Stars)
		}
		line += fit(st.textS.Render(r.Language), langW) + "  " +
			fit(st.warn.Render(stars), starW) + "  " +
			fit(st.agoStyle(r.PushedAt).Render(ago(r.PushedAt)), agoW)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

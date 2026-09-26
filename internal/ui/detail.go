package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bferg314/folgit/internal/gitinfo"
)

// Below this width the pane can't sit beside the list; "i" then shows it
// full-screen instead.
const sidePaneMinWidth = 100

type detailLayout int

const (
	layoutNone detailLayout = iota
	layoutSide
	layoutFull
)

type detailState struct {
	show  bool // side pane enabled (wide terminals)
	full  bool // full-screen pane (narrow terminals)
	path  string
	seq   int
	cache map[string]*gitinfo.Details
}

type (
	detailTickMsg struct {
		seq  int
		path string
	}
	detailMsg struct {
		path    string
		details gitinfo.Details
	}
)

func (a *App) detailLayout() detailLayout {
	switch {
	case a.tab != tabLocal:
		return layoutNone
	case a.w >= sidePaneMinWidth && a.detail.show:
		return layoutSide
	case a.w < sidePaneMinWidth && a.detail.full:
		return layoutFull
	}
	return layoutNone
}

func (a *App) toggleDetails() {
	if a.w >= sidePaneMinWidth {
		a.detail.show = !a.detail.show
	} else {
		a.detail.full = !a.detail.full
	}
}

// syncDetails schedules a load when the selection moves to a repo whose
// details aren't cached. The short delay means scrolling quickly through
// the list doesn't spawn git for every row passed over.
func (a *App) syncDetails() tea.Cmd {
	if a.detailLayout() == layoutNone {
		return nil
	}
	r := a.local.selected()
	if r == nil || r.path == a.detail.path {
		return nil
	}
	a.detail.path = r.path
	if a.detail.cache[r.path] != nil {
		return nil
	}
	a.detail.seq++
	seq, path := a.detail.seq, r.path
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return detailTickMsg{seq: seq, path: path} })
}

func loadDetails(path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return detailMsg{path: path, details: gitinfo.GetDetails(ctx, path, 10, 8, 60)}
	}
}

// detailLoading reports whether the visible pane is waiting for data.
func (a *App) detailLoading() bool {
	r := a.local.selected()
	return a.detailLayout() != layoutNone && r != nil && a.detail.cache[r.path] == nil
}

// invalidateDetails drops cached details for path so they reload.
func (a *App) invalidateDetails(path string) {
	delete(a.detail.cache, path)
	if a.detail.path == path {
		a.detail.path = ""
	}
}

func (a *App) renderLocalBody(h int) string {
	switch a.detailLayout() {
	case layoutSide:
		pw := max(40, min(70, a.w*2/5))
		lw := a.w - pw
		return lipgloss.JoinHorizontal(lipgloss.Top,
			padLines(a.renderLocal(lw, h), h, lw),
			padLines(a.renderDetail(pw, h, true), h, pw))
	case layoutFull:
		return a.renderDetail(a.w, h, false)
	}
	return a.renderLocal(a.w, h)
}

func (a *App) renderDetail(w, h int, border bool) string {
	st := a.st
	r := a.local.selected()
	cw := w - 3 // content width after the border/gutter
	var lines []string
	add := func(s string) { lines = append(lines, fit(s, cw)) }
	section := func(title string) {
		lines = append(lines, "")
		add(st.colHead.Render(title))
	}

	if r == nil {
		add(st.dim.Render("No repository selected"))
	} else {
		add(st.selName.Render(r.rel))
		if s := r.status; s != nil && s.Err == nil {
			line := st.branch.Render(branchLabel(s))
			if s.Upstream != "" {
				line += st.dim.Render(" → " + s.Upstream)
			} else if s.Branch != "" {
				line += st.dim.Render("  no upstream")
			}
			add(line)
			add(st.changesLine(s))
		} else if s != nil {
			add(st.bad.Render(s.Err.Error()))
		}

		d := a.detail.cache[r.path]
		if d == nil {
			lines = append(lines, "")
			add(a.spin.View() + st.dim.Render(" loading…"))
		} else {
			a.appendDetails(d, cw, add, section)
		}
	}

	if len(lines) > h {
		lines = lines[:h]
	}
	prefix := " "
	if border {
		prefix = st.rule.Render("│") + " "
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = prefix + fit(l, cw)
	}
	return strings.Join(lines, "\n")
}

func (a *App) appendDetails(d *gitinfo.Details, cw int, add func(string), section func(string)) {
	st := a.st

	if len(d.Commits) > 0 {
		section("RECENT COMMITS")
		for _, c := range d.Commits {
			when := st.agoStyle(c.When).Render(fmt.Sprintf("%8s", ago(c.When)))
			subjW := max(8, cw-8-1-9-1)
			add(st.warn.Render(fmt.Sprintf("%-7s", c.Hash)) + " " + fit(st.textS.Render(c.Subject), subjW) + " " + when)
		}
	}

	if len(d.Branches) > 0 {
		section("BRANCHES")
		for _, b := range d.Branches {
			mark, name := "  ", st.textS.Render(b.Name)
			if r := a.local.selected(); r != nil && r.status != nil && r.status.Branch == b.Name {
				mark, name = st.marker.Render("* "), st.branch.Render(b.Name)
			}
			var extra string
			switch {
			case b.Track == "gone":
				extra = st.bad.Render("upstream gone")
			case b.Track != "":
				extra = st.ahead.Render(b.Track)
			case b.Upstream == "":
				extra = st.dim.Render("local only")
			}
			add(mark + fit(name, max(10, cw/2-2)) + " " + extra)
		}
	}

	if len(d.Remotes) > 0 {
		section("REMOTES")
		for _, r := range d.Remotes {
			add(st.textS.Render(fmt.Sprintf("%-8s", r.Name)) + " " + st.dim.Render(r.URL))
		}
	}

	if len(d.Readme) > 0 {
		section("README")
		for _, l := range d.Readme {
			if t := strings.TrimLeft(l, "#"); t != l {
				add(st.bold.Render(strings.TrimSpace(t)))
			} else {
				add(st.dim.Render(l))
			}
		}
	}
}

// changesLine spells out the working tree state in words.
func (s styles) changesLine(g *gitinfo.Status) string {
	var parts []string
	add := func(n int, word string, st lipgloss.Style) {
		if n > 0 {
			parts = append(parts, st.Render(fmt.Sprintf("%d %s", n, word)))
		}
	}
	add(g.Conflicts, "conflicted", s.bad)
	add(g.Staged, "staged", s.ok)
	add(g.Unstaged, "modified", s.warn)
	add(g.Untracked, "untracked", s.dim)
	add(g.Ahead, "to push", s.ahead)
	add(g.Behind, "to pull", s.behind)
	add(g.Stashes, "stashed", s.dim)
	if len(parts) == 0 {
		return s.ok.Render("✓ clean and in sync")
	}
	return strings.Join(parts, s.faintText.Render(" · "))
}

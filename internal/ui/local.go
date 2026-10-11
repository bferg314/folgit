package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/launcher"
	"github.com/bferg314/folgit/internal/repos"
)

type sortMode int

const (
	sortRecent sortMode = iota
	sortName
	sortDirty
)

func (s sortMode) String() string { return [...]string{"recent", "name", "dirty"}[s] }

type localRepo struct {
	path    string
	rel     string // slash-separated path relative to the root
	status  *gitinfo.Status
	busy    string // e.g. "pulling"
	gen     int
	matches []int
	// pinned and hidden mirror the config; refresh keeps them current.
	pinned, hidden bool
}

type localTab struct {
	cfg      *config.Config
	rows     map[string]*localRepo
	view     []*localRepo
	cursor   int
	offset   int
	sort     sortMode
	filter   textinput.Model
	scanning bool
	events   <-chan repos.Event
	// showHidden lists hidden repos too, marked as hidden.
	showHidden bool
}

func newLocalTab(cfg *config.Config) localTab {
	return localTab{cfg: cfg, rows: make(map[string]*localRepo), filter: newFilter()}
}

func newFilter() textinput.Model {
	f := textinput.New()
	f.Prompt = ""
	f.Placeholder = "type to filter…"
	return f
}

func (t *localTab) add(path, root string, gen int) *localRepo {
	r := t.rows[path]
	if r == nil {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			rel = filepath.Base(path)
		}
		r = &localRepo{path: path, rel: filepath.ToSlash(rel)}
		t.rows[path] = r
	}
	r.gen = gen
	return r
}

func (t *localTab) apply(events []repos.Event, root string, gen int) {
	for _, e := range events {
		r := t.add(e.Path, root, gen)
		if e.Status != nil {
			r.status = e.Status
		}
	}
}

// prune drops repos not seen in the latest scan (deleted or moved).
func (t *localTab) prune(gen int) {
	for p, r := range t.rows {
		if r.gen != gen {
			delete(t.rows, p)
		}
	}
}

func (t *localTab) anyBusy() bool {
	for _, r := range t.rows {
		if r.busy != "" {
			return true
		}
	}
	return false
}

func (t *localTab) selected() *localRepo {
	if t.cursor >= 0 && t.cursor < len(t.view) {
		return t.view[t.cursor]
	}
	return nil
}

// refresh rebuilds the visible list, keeping the cursor on the same repo.
func (t *localTab) refresh() {
	var keep string
	if r := t.selected(); r != nil {
		keep = r.path
	}

	pinned, hidden := t.cfg.RepoMarks()
	all := make([]*localRepo, 0, len(t.rows))
	for _, r := range t.rows {
		r.matches = nil
		key := config.RepoKey(r.path)
		r.pinned, r.hidden = pinned[key], hidden[key]
		if !r.hidden || t.showHidden {
			all = append(all, r)
		}
	}
	sort.Slice(all, func(i, j int) bool { return t.less(all[i], all[j]) })

	if q := t.filter.Value(); q != "" {
		names := make([]string, len(all))
		for i, r := range all {
			names[i] = r.rel
		}
		ms := fuzzy.Find(q, names)
		view := make([]*localRepo, len(ms))
		for i, m := range ms {
			view[i] = all[m.Index]
			view[i].matches = m.MatchedIndexes
		}
		all = view
	}
	t.view = all

	t.cursor = min(t.cursor, max(0, len(t.view)-1))
	for i, r := range t.view {
		if r.path == keep {
			t.cursor = i
			break
		}
	}
}

// less puts pinned repos first, then orders by the chosen sort.
func (t *localTab) less(a, b *localRepo) bool {
	if a.pinned != b.pinned {
		return a.pinned
	}
	switch t.sort {
	case sortDirty:
		ad, bd := a.status != nil && a.status.Dirty(), b.status != nil && b.status.Dirty()
		if ad != bd {
			return ad
		}
		fallthrough
	case sortRecent:
		at, bt := lastCommit(a), lastCommit(b)
		if !at.Equal(bt) {
			return at.After(bt)
		}
	}
	return strings.ToLower(a.rel) < strings.ToLower(b.rel)
}

func lastCommit(r *localRepo) (t0 time.Time) {
	if r.status != nil {
		return r.status.LastCommit
	}
	return t0
}

// hiddenCount is how many repos are hidden, shown or not.
func (t *localTab) hiddenCount() int {
	n := 0
	for _, r := range t.rows {
		if r.hidden {
			n++
		}
	}
	return n
}

func (t *localTab) move(delta int) {
	t.cursor = max(0, min(len(t.view)-1, t.cursor+delta))
}

// ---- keys ----

func (a *App) localKey(key string) tea.Cmd {
	if a.detailScrollKey(key) {
		return nil
	}
	t := &a.local
	page := max(1, a.h-8)
	switch key {
	case "up", "k":
		t.move(-1)
	case "down", "j":
		t.move(1)
	case "pgup":
		t.move(-page)
	case "pgdown":
		t.move(page)
	case "home":
		t.cursor = 0
	case "end":
		t.cursor = max(0, len(t.view)-1)
	case "s":
		t.sort = (t.sort + 1) % 3
		t.refresh()
	case "r":
		return a.startScan()
	case "F":
		return a.startBulk(false)
	case "P":
		return a.startBulk(true)
	case "d":
		a.toggleDetails()
	case "b":
		return a.openCleanup(false)
	case "B":
		return a.openCleanup(true)
	case "D":
		return a.openRemove()
	case "w":
		return a.localWeb()
	case "i":
		return a.localIssues()
	case "*":
		return a.togglePinned()
	case "h":
		return a.toggleHidden()
	case "H":
		return a.toggleShowHidden()
	case "g":
		r := t.selected()
		switch {
		case r == nil:
			return nil
		case a.cwdFile == "":
			return a.notify(2, "Shell integration isn't set up: run `folgit init <shell>` (see README)")
		}
		a.cdTarget = r.path
		return tea.Quit
	case "p":
		r := t.selected()
		switch {
		case r == nil || r.busy != "":
			return nil
		case r.status == nil || r.status.Upstream == "":
			return a.notify(2, "%s has no upstream to pull from", r.rel)
		}
		r.busy = "pulling"
		path := r.path
		return tea.Batch(a.startSpinner(), func() tea.Msg {
			return pullMsg{path: path, err: launcher.Pull(context.Background(), path)}
		})
	case "enter":
		if r := t.selected(); r != nil {
			tools := a.enabledTools()
			if len(tools) == 0 {
				return a.notify(2, "No tools enabled — see the Settings tab")
			}
			a.popup = &toolPopup{repo: r, tools: tools}
		}
	default:
		if r := t.selected(); r != nil && !reservedKeys[key] {
			for _, tool := range a.enabledTools() {
				if tool.Key == key {
					return a.launch(tool, r.path)
				}
			}
		}
	}
	return nil
}

// togglePinned pins or unpins the selected repo.
func (a *App) togglePinned() tea.Cmd {
	r := a.local.selected()
	if r == nil {
		return nil
	}
	a.cfg.SetPinned(r.path, !r.pinned)
	a.local.refresh()
	msg := "Pinned %s to the top"
	if !r.pinned {
		msg = "Unpinned %s"
	}
	return tea.Batch(a.saveConfig(), a.notify(1, msg, r.rel))
}

// toggleHidden hides the selected repo, or unhides it when hidden repos
// are being shown.
func (a *App) toggleHidden() tea.Cmd {
	r := a.local.selected()
	if r == nil {
		return nil
	}
	a.cfg.SetHidden(r.path, !r.hidden)
	a.local.refresh()
	var note tea.Cmd
	switch {
	case !r.hidden:
		note = a.notify(1, "%s is no longer hidden", r.rel)
	case a.local.showHidden:
		note = a.notify(1, "Hid %s", r.rel)
	default:
		note = a.notify(1, "Hid %s · H shows hidden repos", r.rel)
	}
	return tea.Batch(a.saveConfig(), note)
}

// toggleShowHidden shows or hides the hidden repos in the list.
func (a *App) toggleShowHidden() tea.Cmd {
	t := &a.local
	n := t.hiddenCount()
	if n == 0 && !t.showHidden {
		return a.notify(0, "No hidden repos: h hides the selected one")
	}
	t.showHidden = !t.showHidden
	t.refresh()
	if t.showHidden {
		return a.notify(0, "Showing %d hidden %s · h unhides · H hides them again", n, plural(n, "repo", "repos"))
	}
	return a.notify(0, "Hiding %d %s", n, plural(n, "repo", "repos"))
}

// ---- view ----

func (a *App) renderLocal(w, h int) string {
	t := &a.local
	st := a.st
	var lines []string

	if t.filter.Focused() || t.filter.Value() != "" {
		lines = append(lines, " "+st.key.Render("/")+" "+t.filter.View())
		h--
	}

	if len(t.view) == 0 {
		msg := "No git repositories found here."
		switch {
		case t.scanning:
			msg = "Scanning for repositories…"
		case t.filter.Value() != "":
			msg = "Nothing matches that filter."
		case len(t.rows) > 0:
			msg = "Every repo here is hidden. Press H to show them."
		}
		lines = append(lines, "", "   "+st.dim.Render(msg))
		return strings.Join(lines, "\n")
	}

	// Column widths. Status and branch take what their contents need; the
	// repo name has priority over the branch when space is short.
	const agoW, minBranchW = 9, 12
	statusW, branchW, wantName := len("STATUS"), len("BRANCH"), len("REPOSITORY")
	markW := 2 // cursor marker
	for _, r := range t.view {
		if r.pinned || r.hidden {
			markW = 4 // and a pin or hidden flag
		}
		wantName = max(wantName, utf8.RuneCountInString(r.rel))
		switch {
		case r.busy != "":
			statusW = max(statusW, 2+len(r.busy))
		case r.status != nil && r.status.Err == nil:
			branchW = max(branchW, utf8.RuneCountInString(branchLabel(r.status)))
			statusW = max(statusW, lipgloss.Width(st.statusCell(r.status)))
		}
	}
	statusW = min(statusW, 16)
	branchW = min(branchW, 28)
	avail := w - markW - statusW - agoW - 6 // three gaps
	if wantName+branchW > avail {
		branchW = max(min(branchW, minBranchW), avail-wantName)
	}
	nameW := max(10, avail-branchW)

	lines = append(lines, strings.Repeat(" ", markW)+
		fit(st.colHead.Render("REPOSITORY"), nameW)+"  "+
		fit(st.colHead.Render("BRANCH"), branchW)+"  "+
		fit(st.colHead.Render("STATUS"), statusW)+"  "+
		fit(st.colHead.Render("COMMITTED"), agoW))
	h--

	// Keep the cursor in view.
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+h {
		t.offset = t.cursor - h + 1
	}
	t.offset = max(0, min(t.offset, len(t.view)-h))

	for i := t.offset; i < min(len(t.view), t.offset+h); i++ {
		r := t.view[i]
		sel := i == t.cursor
		marker := "  "
		if sel {
			marker = st.marker.Render("▌") + " "
		}
		if markW > 2 {
			switch {
			case r.pinned:
				marker += st.pin.Render("★") + " "
			case r.hidden:
				marker += st.dim.Render("⊘") + " "
			default:
				marker += "  "
			}
		}
		name := st.highlightName(r.rel, nameW, r.matches, sel)
		if r.hidden && !sel {
			name = st.dim.Render(ansi.Strip(name))
		}

		var branch, status, when string
		switch {
		case r.status == nil:
			branch = st.faintText.Render("…")
			status = a.spin.View()
		case r.status.Err != nil:
			branch = st.faintText.Render("—")
			status = st.bad.Render("error")
		default:
			if r.status.Branch == "" {
				branch = st.warn.Render(branchLabel(r.status))
			} else {
				branch = st.branch.Render(branchLabel(r.status))
			}
			status = st.statusCell(r.status)
			when = st.agoStyle(r.status.LastCommit).Render(ago(r.status.LastCommit))
		}
		if r.busy != "" {
			status = a.spin.View() + " " + st.dim.Render(r.busy)
		}

		lines = append(lines, marker+name+"  "+fit(branch, branchW)+"  "+fit(status, statusW)+"  "+fit(when, agoW))
	}
	return strings.Join(lines, "\n")
}

func branchLabel(s *gitinfo.Status) string {
	if s.Branch != "" {
		return s.Branch
	}
	if s.Head != "" {
		return "@" + s.Head
	}
	return "(empty)"
}

func (s styles) statusCell(g *gitinfo.Status) string {
	var parts []string
	if g.Conflicts > 0 {
		parts = append(parts, s.bad.Render(fmt.Sprintf("!%d", g.Conflicts)))
	}
	if n := g.Staged + g.Unstaged + g.Untracked; n > 0 {
		parts = append(parts, s.warn.Render(fmt.Sprintf("●%d", n)))
	}
	if g.Ahead > 0 {
		parts = append(parts, s.ahead.Render(fmt.Sprintf("↑%d", g.Ahead)))
	}
	if g.Behind > 0 {
		parts = append(parts, s.behind.Render(fmt.Sprintf("↓%d", g.Behind)))
	}
	if g.Stashes > 0 {
		parts = append(parts, s.dim.Render(fmt.Sprintf("≡%d", g.Stashes)))
	}
	if g.Upstream == "" && g.Branch != "" {
		parts = append(parts, s.dim.Render("∅"))
	}
	if len(parts) == 0 {
		return s.ok.Render("✓")
	}
	return strings.Join(parts, " ")
}

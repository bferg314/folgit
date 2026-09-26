package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/launcher"
)

// cleanupRow is one stale branch in one repo.
type cleanupRow struct {
	path, rel string
	base      string // what it was compared against, e.g. "origin/main"
	branch    gitinfo.StaleBranch
	selected  bool
	deleted   bool
	err       string
}

// cleanupView is the branch cleanup popup, for one repo or all of them.
type cleanupView struct {
	title    string
	paths    []string // repos being checked
	multi    bool     // show the repo column
	loading  bool
	fetching bool
	deleting bool
	confirm  bool
	rows     []cleanupRow
	failed   []string // repos that couldn't be checked
	cursor   int
	offset   int
}

type (
	cleanupScanMsg struct {
		view   *cleanupView
		rows   []cleanupRow
		failed []string
	}
	cleanupDoneMsg struct {
		view    *cleanupView
		results map[int]error // row index -> error (nil = deleted)
	}
)

// openCleanup starts the popup for the selected repo, or for every repo.
func (a *App) openCleanup(all bool) tea.Cmd {
	v := &cleanupView{multi: all}
	if all {
		for p := range a.local.rows {
			v.paths = append(v.paths, p)
		}
		sort.Strings(v.paths)
		v.title = fmt.Sprintf("%d repos", len(v.paths))
	} else {
		r := a.local.selected()
		if r == nil {
			return nil
		}
		v.paths = []string{r.path}
		v.title = r.rel
	}
	if len(v.paths) == 0 {
		return nil
	}
	a.cleanup = v
	return a.scanCleanup(false)
}

// scanCleanup finds stale branches in the popup's repos, optionally after
// fetching with --prune so branches deleted on the remote show as gone.
func (a *App) scanCleanup(fetch bool) tea.Cmd {
	v := a.cleanup
	v.loading, v.fetching, v.confirm = true, fetch, false
	rels := map[string]string{}
	for _, p := range v.paths {
		if r := a.local.rows[p]; r != nil {
			rels[p] = r.rel
		}
	}
	return tea.Batch(a.startSpinner(), func() tea.Msg {
		var (
			mu     sync.Mutex
			wg     sync.WaitGroup
			rows   []cleanupRow
			failed []string
		)
		for _, p := range v.paths {
			wg.Add(1)
			go func() {
				defer wg.Done()
				bulkSlots <- struct{}{}
				defer func() { <-bulkSlots }()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				if fetch {
					// A failed fetch still leaves local merge info useful.
					_ = launcher.Fetch(ctx, p)
				}
				branches, base, err := gitinfo.StaleBranches(ctx, p)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed = append(failed, rels[p])
					return
				}
				for _, b := range branches {
					// Merged branches are safe; unmerged ones need an
					// explicit choice.
					rows = append(rows, cleanupRow{path: p, rel: rels[p], base: base, branch: b, selected: b.Merged})
				}
			}()
		}
		wg.Wait()
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].rel != rows[j].rel {
				return rows[i].rel < rows[j].rel
			}
			return rows[i].branch.Name < rows[j].branch.Name
		})
		sort.Strings(failed)
		return cleanupScanMsg{view: v, rows: rows, failed: failed}
	})
}

func (a *App) handleCleanupScan(msg cleanupScanMsg) {
	v := a.cleanup
	if v == nil || v != msg.view {
		return
	}
	v.loading, v.fetching = false, false
	v.rows, v.failed = msg.rows, msg.failed
	v.cursor = min(v.cursor, max(0, len(v.rows)-1))
}

// deleteSelected removes the selected branches, one repo at a time (git
// takes a lock per repo) but repos in parallel.
func (a *App) deleteSelected() tea.Cmd {
	v := a.cleanup
	byRepo := map[string][]int{}
	for i, r := range v.rows {
		if r.selected && !r.deleted {
			byRepo[r.path] = append(byRepo[r.path], i)
		}
	}
	if len(byRepo) == 0 {
		return nil
	}
	v.deleting, v.confirm = true, false
	rows := v.rows
	return tea.Batch(a.startSpinner(), func() tea.Msg {
		var (
			mu      sync.Mutex
			wg      sync.WaitGroup
			results = map[int]error{}
		)
		for path, idx := range byRepo {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				for _, i := range idx {
					err := gitinfo.DeleteBranch(ctx, path, rows[i].branch.Name)
					mu.Lock()
					results[i] = err
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		return cleanupDoneMsg{view: v, results: results}
	})
}

func (a *App) handleCleanupDone(msg cleanupDoneMsg) tea.Cmd {
	v := msg.view
	v.deleting = false
	deleted, failed := 0, 0
	touched := map[string]bool{}
	var restore []string
	for i, err := range msg.results {
		r := &v.rows[i]
		touched[r.path] = true
		if err != nil {
			r.err = err.Error()
			failed++
			continue
		}
		r.deleted, r.selected, r.err = true, false, ""
		deleted++
		restore = append(restore, fmt.Sprintf("%s %s", r.branch.Name, r.branch.Hash))
	}
	cmds := []tea.Cmd{a.saveCache()}
	for p := range touched {
		a.invalidateDetails(p)
		cmds = append(cmds, refreshStatus(p))
	}
	kind, text := 1, fmt.Sprintf("Deleted %d branch(es)", deleted)
	if failed > 0 {
		kind, text = 2, fmt.Sprintf("%s, %d failed", text, failed)
	}
	if len(restore) == 1 {
		// Enough to undo with `git branch <name> <hash>`.
		text += " · restore: git branch " + restore[0]
	}
	cmds = append(cmds, a.notify(kind, "%s", text))
	return tea.Batch(cmds...)
}

func (a *App) cleanupKey(key string) tea.Cmd {
	v := a.cleanup
	if v.deleting {
		return nil
	}
	if v.confirm {
		switch key {
		case "y":
			return a.deleteSelected()
		default:
			v.confirm = false
		}
		return nil
	}
	page := max(1, a.popupListHeight())
	move := func(d int) { v.cursor = max(0, min(len(v.rows)-1, v.cursor+d)) }
	switch key {
	case "esc", "q", "b", "B":
		a.cleanup = nil
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
		move(len(v.rows))
	case "space":
		if v.cursor < len(v.rows) && !v.rows[v.cursor].deleted {
			v.rows[v.cursor].selected = !v.rows[v.cursor].selected
			move(1)
		}
	case "a":
		all := true
		for _, r := range v.rows {
			all = all && (r.selected || r.deleted)
		}
		for i := range v.rows {
			if !v.rows[i].deleted {
				v.rows[i].selected = !all
			}
		}
	case "f":
		if !v.loading {
			return a.scanCleanup(true)
		}
	case "enter", "d":
		if !v.loading && v.selectedCount() > 0 {
			v.confirm = true
		}
	}
	return nil
}

func (v *cleanupView) selectedCount() (n int) {
	for _, r := range v.rows {
		if r.selected && !r.deleted {
			n++
		}
	}
	return n
}

// riskyCount is how many selected branches have commits not in the default
// branch; deleting those can lose work.
func (v *cleanupView) riskyCount() (n int) {
	for _, r := range v.rows {
		if r.selected && !r.deleted && !r.branch.Merged {
			n++
		}
	}
	return n
}

func (a *App) renderCleanup() string {
	st := a.st
	v := a.cleanup
	bw, _ := a.popupBoxSize()
	cw := bw - 2 - 4
	listH := a.popupListHeight()

	title := st.boxTitle.Render("Branch cleanup") + st.dim.Render(" · ") + st.bold.Render(v.title)
	var right string
	switch {
	case v.loading || v.deleting:
		right = a.spin.View()
	default:
		right = st.dim.Render(fmt.Sprintf("%d selected", v.selectedCount()))
	}
	gap := max(1, cw-lipgloss.Width(title)-lipgloss.Width(right))
	lines := []string{fit(title+strings.Repeat(" ", gap)+right, cw), ""}

	// Keep room for the "couldn't check" line so a long list can't hide it.
	rowsH := listH
	if len(v.failed) > 0 && !v.loading {
		rowsH = max(1, listH-2)
	}

	var body []string
	switch {
	case v.loading && v.fetching:
		body = []string{a.spin.View() + st.dim.Render(" Fetching and checking branches…")}
	case v.loading:
		body = []string{a.spin.View() + st.dim.Render(" Checking branches…")}
	case len(v.rows) == 0:
		body = []string{st.ok.Render("✓ ") + st.dim.Render("No merged or orphaned branches. Press ") + st.key.Render("f") +
			st.dim.Render(" to fetch first, so branches deleted on GitHub show up.")}
	default:
		body = a.cleanupRows(cw, rowsH)
	}
	if len(v.failed) > 0 && !v.loading {
		body = append(body, "", st.warn.Render("Couldn't check: ")+st.dim.Render(strings.Join(v.failed, ", ")))
	}
	for len(body) < listH {
		body = append(body, "")
	}
	lines = append(lines, body[:listH]...)

	var hint string
	if v.confirm {
		n, risky := v.selectedCount(), v.riskyCount()
		hint = st.warn.Render(fmt.Sprintf("Delete %d branch(es)?", n))
		if risky > 0 {
			hint += " " + st.bad.Render(fmt.Sprintf("%d have commits that aren't in the default branch.", risky))
		}
		hint += "  " + st.key.Render("y") + st.dim.Render(" delete · any other key cancels")
	} else {
		hint = st.key.Render("space") + st.dim.Render(" select · ") + st.key.Render("a") + st.dim.Render(" all · ") +
			st.key.Render("enter") + st.dim.Render(" delete · ") + st.key.Render("f") + st.dim.Render(" fetch & recheck · ") +
			st.key.Render("esc") + st.dim.Render(" close")
	}
	lines = append(lines, "", fit(hint, cw))
	return st.box.Width(bw).Render(strings.Join(lines, "\n"))
}

func (a *App) cleanupRows(cw, h int) []string {
	st := a.st
	v := a.cleanup
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+h {
		v.offset = v.cursor - h + 1
	}
	v.offset = max(0, min(v.offset, len(v.rows)-h))

	const agoW = 8
	repoW := 0
	if v.multi {
		for _, r := range v.rows {
			repoW = max(repoW, len([]rune(r.rel)))
		}
		repoW = min(repoW, cw/4)
	}
	branchW := 0
	for _, r := range v.rows {
		branchW = max(branchW, len([]rune(r.branch.Name)))
	}
	branchW = min(branchW, cw/3)
	reasonW := max(10, cw-2-2-repoW-branchW-agoW-4)
	if v.multi {
		reasonW -= 2
	}

	var rows []string
	for i := v.offset; i < min(len(v.rows), v.offset+h); i++ {
		r := v.rows[i]
		sel := i == v.cursor
		marker := "  "
		if sel {
			marker = st.marker.Render("▌") + " "
		}
		check := st.faintText.Render("○ ")
		switch {
		case r.deleted:
			check = st.ok.Render("✓ ")
		case r.err != "":
			check = st.bad.Render("✗ ")
		case r.selected:
			check = st.key.Render("◉ ")
		}

		line := marker + check
		if v.multi {
			line += st.highlightName(r.rel, repoW, nil, false) + "  "
		}
		name := st.branch.Render(r.branch.Name)
		switch {
		case r.deleted:
			name = st.faintText.Render(r.branch.Name)
		case sel:
			name = st.selName.Render(r.branch.Name)
		}
		line += fit(name, branchW) + "  "

		var reason string
		switch {
		case r.deleted:
			reason = st.dim.Render("deleted · was " + r.branch.Hash)
		case r.err != "":
			reason = st.bad.Render(r.err)
		case r.branch.Merged && r.branch.Gone:
			reason = st.ok.Render("merged into "+r.base) + st.dim.Render(" · remote deleted")
		case r.branch.Merged:
			reason = st.ok.Render("merged into " + r.base)
		case r.branch.Unmerged > 0:
			reason = st.warn.Render("remote deleted") + st.dim.Render(" · ") +
				st.bad.Render(fmt.Sprintf("%d commit(s) not in %s", r.branch.Unmerged, r.base))
		default:
			reason = st.warn.Render("remote deleted")
		}
		line += fit(reason, reasonW) + "  " + fit(st.agoStyle(r.branch.When).Render(ago(r.branch.When)), agoW)
		rows = append(rows, line)
	}
	return rows
}

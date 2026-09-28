package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/gitinfo"
)

func markApp(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	a := New(config.Default(), filepath.Join(t.TempDir(), "config.toml"), root, nil, "")
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 14})
	for i, n := range []string{"newest", "middle", "oldest"} {
		r := a.local.add(filepath.Join(root, n), root, 0)
		r.status = &gitinfo.Status{Branch: "main", Upstream: "origin/main", Behind: 1, Remotes: []string{"https://github.com/o/" + n},
			LastCommit: time.Now().Add(-time.Duration(i) * time.Hour)}
	}
	a.local.refresh()
	return a, root
}

func viewNames(a *App) []string {
	var names []string
	for _, r := range a.local.view {
		names = append(names, r.rel)
	}
	return names
}

func selectRepo(t *testing.T, a *App, rel string) {
	t.Helper()
	for i, r := range a.local.view {
		if r.rel == rel {
			a.local.cursor = i
			return
		}
	}
	t.Fatalf("%s not in view %v", rel, viewNames(a))
}

func TestPinnedReposComeFirst(t *testing.T) {
	a, _ := markApp(t)
	selectRepo(t, a, "oldest")
	a.localKey("*")

	for _, mode := range []sortMode{sortRecent, sortName, sortDirty} {
		a.local.sort = mode
		a.local.refresh()
		if got := viewNames(a); got[0] != "oldest" {
			t.Errorf("sort %s: pinned repo should be first: %v", mode, got)
		}
	}
	if len(a.cfg.Pinned) != 1 {
		t.Errorf("the pin should be saved in the config: %q", a.cfg.Pinned)
	}
	if !strings.Contains(ansi.Strip(a.render()), "★ oldest") {
		t.Errorf("pinned repos are marked with a star:\n%s", ansi.Strip(a.render()))
	}

	a.localKey("*")
	if len(a.cfg.Pinned) != 0 || viewNames(a)[0] == "oldest" {
		t.Errorf("unpinning restores the sort: %v %q", viewNames(a), a.cfg.Pinned)
	}
}

func TestHiddenReposAreLeftOut(t *testing.T) {
	a, _ := markApp(t)
	selectRepo(t, a, "middle")
	a.localKey("h")

	if got := viewNames(a); len(got) != 2 || strings.Contains(strings.Join(got, " "), "middle") {
		t.Errorf("hidden repo should leave the list: %v", got)
	}
	if r := a.local.selected(); r == nil || r.rel != "oldest" {
		t.Errorf("the cursor should move to the next repo, got %v", r)
	}
	if s := ansi.Strip(a.renderHeader()); !strings.Contains(s, "Local 2") {
		t.Errorf("the tab count leaves hidden repos out: %s", s)
	}
	if s := ansi.Strip(bar(a)); !strings.Contains(s, "↓2 to pull") {
		t.Errorf("status bar totals leave hidden repos out: %s", s)
	}

	a.startBulk(false)
	if r := a.local.rows[filepath.Join(a.root, "middle")]; r.busy != "" || a.bulk.total != 2 {
		t.Errorf("fetch all skips hidden repos: busy %q total %d", r.busy, a.bulk.total)
	}
	a.bulk = nil

	a.localKey("H")
	if got := viewNames(a); len(got) != 3 {
		t.Errorf("H shows hidden repos: %v", got)
	}
	if !strings.Contains(ansi.Strip(a.render()), "⊘ middle") {
		t.Errorf("hidden repos are marked when shown:\n%s", ansi.Strip(a.render()))
	}
	selectRepo(t, a, "middle")
	a.localKey("h")
	if len(a.cfg.Hidden) != 0 {
		t.Errorf("h on a shown hidden repo unhides it: %q", a.cfg.Hidden)
	}
}

func TestEveryRepoHidden(t *testing.T) {
	a, _ := markApp(t)
	for range 3 {
		a.localKey("h")
	}
	if s := ansi.Strip(a.render()); !strings.Contains(s, "Press H to show them") {
		t.Errorf("an all-hidden list should say how to get them back:\n%s", s)
	}
}

package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/gitinfo"
)

// newDetailApp builds an app with two repos, the first with 40 commits of
// cached details, at the given terminal size.
func newDetailApp(t *testing.T, w, h int) *App {
	t.Helper()
	a := New(config.Default(), "cfg", "/root", nil, "")
	a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	now := time.Now()
	for _, name := range []string{"alpha", "beta"} {
		r := a.local.add("/root/"+name, "/root", 0)
		r.status = &gitinfo.Status{Branch: "main", LastCommit: now}
	}
	a.local.sort = sortName
	a.local.refresh()

	var d gitinfo.Details
	for i := range 40 {
		d.Commits = append(d.Commits, gitinfo.Commit{Hash: fmt.Sprintf("c%06d", i), Subject: fmt.Sprintf("commit number %d", i), When: now})
	}
	a.detail.cache["/root/alpha"] = &d
	a.detail.cache["/root/beta"] = &gitinfo.Details{}
	return a
}

func key(k string) tea.KeyPressMsg {
	switch k {
	case "ctrl+d":
		return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

func screen(a *App) string { return ansi.Strip(a.render()) }

func TestDetailScrollKeys(t *testing.T) {
	a := newDetailApp(t, 140, 20)
	s := screen(a)
	if !strings.Contains(s, "commit number 0") || !strings.Contains(s, "J/K ↕ 0%") {
		t.Fatalf("expected top of details with scroll indicator:\n%s", s)
	}

	a.Update(key("J"))
	a.Update(key("J"))
	s = screen(a)
	if !strings.Contains(s, "alpha") || !strings.Contains(s, "main") {
		t.Fatalf("header should stay pinned while scrolling:\n%s", s)
	}
	if a.detail.scroll != 2 {
		t.Fatalf("scroll = %d, want 2", a.detail.scroll)
	}

	a.Update(key("ctrl+d"))
	if a.detail.scroll <= 2 {
		t.Fatalf("ctrl+d should scroll half a page, scroll = %d", a.detail.scroll)
	}

	for range 100 {
		a.Update(key("J"))
	}
	screen(a)
	if a.detail.scroll != a.detail.maxScroll || !strings.Contains(screen(a), "100%") {
		t.Fatalf("scroll should clamp at the end: scroll=%d max=%d", a.detail.scroll, a.detail.maxScroll)
	}
	if !strings.Contains(screen(a), "commit number 39") {
		t.Fatalf("last commit should be visible at the end:\n%s", screen(a))
	}

	// j moves the list (not the pane) in the side layout, and a new
	// selection starts at the top.
	a.Update(key("j"))
	screen(a)
	if r := a.local.selected(); r == nil || r.rel != "beta" || a.detail.scroll != 0 {
		t.Fatalf("j should select beta and reset scroll; got %v scroll=%d", r, a.detail.scroll)
	}
}

// Bubble Tea renders on a frame timer, so several keys can arrive before
// the next render. Scrolling must not depend on a render having happened.
func TestDetailScrollBeforeRender(t *testing.T) {
	a := newDetailApp(t, 120, 16)
	for range 5 {
		a.Update(key("J"))
	}
	if a.detail.scroll != 5 {
		t.Fatalf("scroll = %d, want 5", a.detail.scroll)
	}
	// The body starts with a blank line and the section title, so five
	// lines down the first visible commit is number 3.
	if s := screen(a); !strings.Contains(s, "commit number 3 ") || strings.Contains(s, "commit number 2 ") {
		t.Fatalf("pane should start at commit 3:\n%s", s)
	}

	// Selecting another repo and scrolling straight away (no render in
	// between) must not carry the old repo's scroll over.
	a.Update(key("j"))
	a.Update(key("J"))
	if a.detail.scroll != 0 {
		t.Fatalf("beta has nothing to scroll, scroll = %d", a.detail.scroll)
	}
}

func TestDetailScrollFullScreen(t *testing.T) {
	a := newDetailApp(t, 80, 20)
	a.Update(key("d"))
	if a.detailLayout() != layoutFull {
		t.Fatal("d should open the full-screen pane on a narrow terminal")
	}
	screen(a)
	a.Update(key("j"))
	a.Update(key("down"))
	screen(a)
	if a.detail.scroll != 2 || a.local.selected().rel != "alpha" {
		t.Fatalf("j/down should scroll the full-screen pane, scroll=%d sel=%s", a.detail.scroll, a.local.selected().rel)
	}
}

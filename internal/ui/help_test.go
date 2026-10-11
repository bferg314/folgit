package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bferg314/folgit/internal/config"
)

func helpApp(t *testing.T, w, h int) *App {
	t.Helper()
	a := New(config.Default(), filepath.Join(t.TempDir(), "config.toml"), t.TempDir(), nil, "")
	a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return a
}

func press(a *App, key string) {
	var msg tea.KeyPressMsg
	switch key {
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		msg = tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	}
	a.Update(msg)
}

func TestHelpFollowsTab(t *testing.T) {
	a := helpApp(t, 120, 30)
	cases := []struct {
		tab       int
		want, not []string
	}{
		{tabLocal, []string{"Everywhere", "Local", "Tools", "Status", "rescan"}, []string{"Remote", "clone selected", "default folder"}},
		{tabRemote, []string{"Everywhere", "Remote", "clone selected", "only my repos"}, []string{"none enabled", "Status", "rescan"}},
		{tabSettings, []string{"Everywhere", "Settings", "default folder"}, []string{"none enabled", "fuzzy filter", "clone selected"}},
	}
	for _, c := range cases {
		a.setTab(c.tab)
		press(a, "?")
		if !a.help {
			t.Fatalf("tab %d: ? didn't open help", c.tab)
		}
		got := ansi.Strip(a.renderHelp())
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("tab %d: help is missing %q:\n%s", c.tab, w, got)
			}
		}
		for _, n := range c.not {
			if strings.Contains(got, n) {
				t.Errorf("tab %d: help shouldn't show %q:\n%s", c.tab, n, got)
			}
		}
		if a.helpScrolls() {
			t.Errorf("tab %d: help should fit in 30 lines, has %d", c.tab, len(a.helpLines()))
		}
		press(a, "x")
		if a.help {
			t.Fatalf("tab %d: a key should close help", c.tab)
		}
	}
}

// Every tab's help fits a standard 80x24 terminal except Local, whose
// stacked columns scroll.
func TestHelpFitsSmallTerminal(t *testing.T) {
	a := helpApp(t, 80, 24)
	for _, tab := range []int{tabRemote, tabSettings} {
		a.setTab(tab)
		if a.helpScrolls() {
			t.Errorf("tab %d: help has %d lines, more than fit in 24", tab, len(a.helpLines()))
		}
	}
}

func TestHelpScrolls(t *testing.T) {
	a := helpApp(t, 80, 15)
	press(a, "?")
	if !a.helpScrolls() {
		t.Fatal("Local help should scroll in a 15-line terminal")
	}
	first := ansi.Strip(a.renderHelp())
	if !strings.Contains(first, "Everywhere") || !strings.Contains(first, "j k scroll") {
		t.Fatalf("unexpected first page:\n%s", first)
	}
	if h := strings.Count(first, "\n") + 1; h > 15 {
		t.Errorf("help is %d lines tall in a 15-line terminal", h)
	}
	for range 100 {
		press(a, "j")
	}
	if !a.help {
		t.Fatal("j should scroll, not close")
	}
	last := ansi.Strip(a.renderHelp())
	if strings.Contains(last, "Everywhere") || !strings.Contains(last, "no upstream") {
		t.Errorf("scrolling down should reach the end:\n%s", last)
	}
	press(a, "x")
	if a.help {
		t.Error("other keys should still close help")
	}
}

func TestHelpInPopup(t *testing.T) {
	a := helpApp(t, 120, 30)
	a.popup = &toolPopup{repo: &localRepo{rel: "r"}, tools: a.enabledTools()}
	press(a, "?")
	got := ansi.Strip(a.renderHelp())
	if !a.help || !strings.Contains(got, "Open in…") || strings.Contains(got, "Everywhere") {
		t.Fatalf("popup help should show only the popup's keys:\n%s", got)
	}
	press(a, "x")
	if a.help || a.popup == nil {
		t.Error("closing help should leave the popup open")
	}
}

// The Local help fits a standard 24-line terminal when it's wide enough
// for two columns, with every default tool enabled.
func TestLocalHelpFits24Lines(t *testing.T) {
	a := helpApp(t, 120, 24)
	for i := range a.cfg.Tools {
		a.cfg.Tools[i].Enabled = true
	}
	if a.helpScrolls() {
		t.Errorf("Local help has %d lines, more than fit in 24", len(a.helpLines()))
	}
}

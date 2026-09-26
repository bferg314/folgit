package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/gitinfo"
)

func TestHighlightNameTruncation(t *testing.T) {
	st := newStyles(true)
	cases := []struct {
		name string
		w    int
		want string
	}{
		{"folgit", 10, "folgit"},
		{"euterpe-solitaire", 17, "euterpe-solitaire"},
		{"euterpe-solitaire", 12, "euterpe-sol…"},
		{"work/clients/very-long-client-project", 30, "…/very-long-client-project"},
		{"work/clients/very-long-client-project", 20, "very-long-client-pr…"},
		{"bferg314/calliope-poker", 18, "…/calliope-poker"},
		{"a/b", 3, "a/b"},
	}
	for _, c := range cases {
		got := strings.TrimRight(ansi.Strip(st.highlightName(c.name, c.w, nil, false)), " ")
		if got != c.want {
			t.Errorf("highlightName(%q, %d) = %q, want %q", c.name, c.w, got, c.want)
		}
		if ansi.StringWidth(st.highlightName(c.name, c.w, nil, false)) != c.w {
			t.Errorf("highlightName(%q, %d) is not exactly %d cells wide", c.name, c.w, c.w)
		}
	}
}

// Fuzzy-match highlights must stay on the right characters after the
// directories are dropped.
func TestHighlightNameMatchesAfterTruncation(t *testing.T) {
	st := newStyles(true)
	name := "work/clients/api"
	// "api" at byte offsets 13, 14, 15.
	out := st.highlightName(name, 8, []int{13, 14, 15}, false)
	if got := strings.TrimRight(ansi.Strip(out), " "); got != "…/api" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(out, st.match.Render("api")) {
		t.Fatalf("match highlight lost: %q", out)
	}
}

// The screenshot case: with the detail pane open and long branch names,
// repo names used to be cut from the left ("…terpe-solitaire").
func TestLocalNamesGetPriorityOverBranches(t *testing.T) {
	a := New(config.Default(), "cfg", "/code", nil, "")
	a.Update(tea.WindowSizeMsg{Width: 130, Height: 20})
	now := time.Now()
	for name, branch := range map[string]string{
		"euterpe-solitaire":  "main",
		"swords-and-arrows":  "main",
		"fuse-tag":           "agent-instructions-and-ci",
		"clock":              "sync-version-and-guard",
		"the-knowledge-base": "main",
	} {
		r := a.local.add("/code/"+name, "/code", 0)
		r.status = &gitinfo.Status{Branch: branch, Upstream: "origin/" + branch, LastCommit: now}
	}
	a.local.refresh()
	a.detail.cache = map[string]*gitinfo.Details{}
	for p := range a.local.rows {
		a.detail.cache[p] = &gitinfo.Details{}
	}

	s := ansi.Strip(a.render())
	for _, want := range []string{"euterpe-solitaire", "swords-and-arrows", "the-knowledge-base", "agent-instruc"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "…terpe") {
		t.Errorf("names must not be cut from the left:\n%s", s)
	}
}

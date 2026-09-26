package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/provider/github"
)

func TestIssuesPopup(t *testing.T) {
	a := newDetailApp(t, 120, 24)
	a.local.selected().status.Remotes = []string{"git@github.com:me/alpha.git"}

	cmd := a.localIssues()
	if a.issues == nil || cmd == nil || !a.issues.loading {
		t.Fatal("i should open the popup and start loading")
	}
	if s := screen(a); !strings.Contains(s, "Issues · me/alpha") || !strings.Contains(s, "Loading issues") {
		t.Fatalf("expected loading popup:\n%s", s)
	}

	// A reply for an older popup is ignored.
	a.Update(issuesMsg{view: &issuesView{}, issues: []github.Issue{{Number: 99}}})
	if len(a.issues.issues) != 0 {
		t.Fatal("stale reply was applied")
	}

	now := time.Now()
	a.Update(issuesMsg{view: a.issues, repo: "me/alpha", more: true, issues: []github.Issue{
		{Number: 12, Title: "Crash when the list is empty", Labels: []string{"bug"}, UpdatedAt: now},
		{Number: 9, Title: "Support GitLab", Labels: []string{"enhancement"}, UpdatedAt: now.Add(-48 * time.Hour)},
	}})
	s := screen(a)
	for _, want := range []string{"#12", "Crash when the list is empty", "bug", "#9", "2+ open", "open in browser"} {
		if !strings.Contains(s, want) {
			t.Errorf("popup missing %q:\n%s", want, s)
		}
	}

	a.Update(key("j"))
	if a.issues.cursor != 1 {
		t.Fatalf("j should move to the second issue, cursor=%d", a.issues.cursor)
	}
	if a.local.selected().rel != "alpha" {
		t.Fatal("keys must not reach the list while the popup is open")
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.issues != nil {
		t.Fatal("esc should close the popup")
	}
}

func TestIssuesErrorsAndNoRemote(t *testing.T) {
	a := newDetailApp(t, 120, 24)

	// alpha has no GitHub remote.
	a.local.selected().status = &gitinfo.Status{Branch: "main", Remotes: []string{"https://gitlab.com/me/alpha"}}
	a.localIssues()
	if a.issues != nil || !strings.Contains(a.toast.text, "no GitHub remote") {
		t.Fatalf("expected a toast, got popup=%v toast=%q", a.issues != nil, a.toast.text)
	}

	a.openIssues("alpha", []string{"me/alpha"})
	a.Update(issuesMsg{view: a.issues, err: github.ErrNotFound})
	if s := screen(a); !strings.Contains(s, "issues are turned off") {
		t.Fatalf("expected not-found explanation:\n%s", s)
	}
	a.Update(issuesMsg{view: a.issues, repo: "me/alpha"})
	if s := screen(a); !strings.Contains(s, "No open issues") {
		t.Fatalf("expected empty state:\n%s", s)
	}
}

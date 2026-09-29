package ui

import (
	"errors"
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

func typeText(a *App, s string) {
	for _, r := range s {
		a.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func TestCreateIssuesBackToBack(t *testing.T) {
	a := newDetailApp(t, 120, 30)
	a.openIssues("alpha", []string{"me/alpha"})
	a.Update(issuesMsg{view: a.issues, repo: "me/alpha", issues: []github.Issue{{Number: 1, Title: "Old one"}}})

	a.Update(key("n"))
	if a.issues.form == nil || !a.issues.form.open {
		t.Fatal("n should open the new issue form")
	}
	if s := screen(a); !strings.Contains(s, "New issue · me/alpha") || !strings.Contains(s, "enter create") {
		t.Fatalf("expected the form:\n%s", s)
	}

	// enter with no title explains instead of sending.
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.issues.form.sending || !strings.Contains(screen(a), "needs a title") {
		t.Fatalf("an empty title must not be sent:\n%s", screen(a))
	}

	typeText(a, "First bug")
	a.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	typeText(a, "details")
	if !a.issues.form.onBody || a.issues.form.body.Value() != "details" {
		t.Fatalf("tab should move to the description, got %q", a.issues.form.body.Value())
	}
	a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if !a.issues.form.sending {
		t.Fatal("ctrl+s should send the issue")
	}
	typeText(a, "ignored")
	if a.issues.form.body.Value() != "details" {
		t.Fatal("the text must not change while it's being sent")
	}

	a.Update(issueCreatedMsg{view: a.issues, issue: github.Issue{Number: 2, Title: "First bug"}})
	f := a.issues.form
	if !f.open || f.title.Value() != "" || f.body.Value() != "" || f.onBody {
		t.Fatalf("after creating, the form clears and stays open on the title: %+v", f)
	}
	if a.issues.issues[0].Number != 2 {
		t.Error("the new issue should be added to the top of the list")
	}

	typeText(a, "Second bug")
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a.Update(issueCreatedMsg{view: a.issues, issue: github.Issue{Number: 3, Title: "Second bug"}})
	if s := screen(a); !strings.Contains(s, "Created #3 Second bug · #2 First bug") {
		t.Fatalf("the form should list what it created:\n%s", s)
	}

	// A failure keeps the text so it can be retried.
	typeText(a, "Third")
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a.Update(issueCreatedMsg{view: a.issues, err: errors.New("github: 403 Forbidden")})
	if f.title.Value() != "Third" || !strings.Contains(screen(a), "403 Forbidden") {
		t.Fatalf("a failed create keeps the draft and shows why:\n%s", screen(a))
	}

	// esc goes back to the list and keeps the draft; q only closes the list.
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.issues == nil || f.open {
		t.Fatal("esc should return to the issue list")
	}
	a.Update(key("n"))
	if f.title.Value() != "Third" {
		t.Error("the draft should survive closing the form")
	}
}

func TestNewIssueNeedsLoadedRepo(t *testing.T) {
	a := newDetailApp(t, 120, 30)
	a.openIssues("alpha", []string{"me/alpha"})
	a.Update(issuesMsg{view: a.issues, err: github.ErrNotFound})
	a.Update(key("n"))
	if a.issues.form != nil {
		t.Fatal("no form without a repo to create in")
	}
}

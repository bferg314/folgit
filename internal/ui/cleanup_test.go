package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/gitinfo"
)

// run executes cmd (and any batch it returns) and feeds the resulting
// messages to a. Timers (spinner, toast expiry) are skipped: commands that
// don't finish within a second are abandoned.
func run(a *App, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		switch m := msg.(type) {
		case nil, spinner.TickMsg:
		case tea.BatchMsg:
			for _, c := range m {
				run(a, c)
			}
		default:
			_, next := a.Update(m)
			run(a, next)
		}
	case <-time.After(time.Second):
	}
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	args = append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)
	out, err := gitinfo.Git(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func TestCleanupPopupDeletesSelected(t *testing.T) {
	t.Setenv("LocalAppData", t.TempDir())
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "main")
	gitT(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	gitT(t, dir, "branch", "done-1")
	gitT(t, dir, "branch", "done-2")
	gitT(t, dir, "checkout", "-q", "-b", "wip")
	gitT(t, dir, "commit", "-q", "--allow-empty", "-m", "wip")
	gitT(t, dir, "checkout", "-q", "main")

	a := New(config.Default(), "cfg", dir, nil, "")
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	r := a.local.add(dir, dir, 0)
	r.status = &gitinfo.Status{Branch: "main"}
	a.local.refresh()

	_, cmd := a.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	run(a, cmd)
	v := a.cleanup
	if v == nil || v.loading || len(v.rows) != 2 {
		t.Fatalf("expected two stale branches, got %+v", v)
	}
	for _, row := range v.rows {
		if !row.selected || !row.branch.Merged {
			t.Fatalf("merged branches should be preselected: %+v", row)
		}
	}
	if s := screen(a); !strings.Contains(s, "Branch cleanup") || !strings.Contains(s, "done-1") || !strings.Contains(s, "merged into main") || strings.Contains(s, "wip") {
		t.Fatalf("unexpected popup:\n%s", s)
	}

	// Deselect done-2, then enter asks for confirmation; n cancels.
	a.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	a.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.confirm || !strings.Contains(screen(a), "Delete 1 branch(es)?") {
		t.Fatalf("enter should ask to confirm:\n%s", screen(a))
	}
	a.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if v.confirm {
		t.Fatal("n should cancel")
	}

	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, cmd = a.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	run(a, cmd)

	branches := gitT(t, dir, "branch", "--format=%(refname:short)")
	if strings.Contains(branches, "done-1") || !strings.Contains(branches, "done-2") || !strings.Contains(branches, "wip") {
		t.Fatalf("only done-1 should be deleted, branches now:\n%s", branches)
	}
	if !v.rows[0].deleted || !strings.Contains(a.toast.text, "Deleted 1 branch") || !strings.Contains(a.toast.text, "git branch done-1") {
		t.Fatalf("row/toast not updated: %+v toast=%q", v.rows[0], a.toast.text)
	}

	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.cleanup != nil {
		t.Fatal("esc should close the popup")
	}
}

func TestCleanupConfirmWarnsAboutUnmergedCommits(t *testing.T) {
	a := newDetailApp(t, 120, 24)
	a.cleanup = &cleanupView{title: "alpha", rows: []cleanupRow{
		{rel: "alpha", base: "origin/main", branch: gitinfo.StaleBranch{Name: "squashed", Gone: true, Unmerged: 3}},
	}}
	s := screen(a)
	if !strings.Contains(s, "3 commit(s) not in origin/main") || !strings.Contains(s, "0 selected") {
		t.Fatalf("unmerged branch should be flagged and not preselected:\n%s", s)
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s := screen(a); !strings.Contains(s, "1 have commits that aren't in the default branch") {
		t.Fatalf("confirm should warn about unmerged commits:\n%s", s)
	}
}

func TestCleanupShowsReposThatFailedEvenWithLongList(t *testing.T) {
	a := newDetailApp(t, 120, 16)
	v := &cleanupView{title: "3 repos", multi: true, failed: []string{"broken-repo"}}
	for i := range 30 {
		v.rows = append(v.rows, cleanupRow{rel: "alpha", base: "main", branch: gitinfo.StaleBranch{Name: "b" + string(rune('a'+i%26)), Merged: true}})
	}
	a.cleanup = v
	if s := screen(a); !strings.Contains(s, "Couldn't check: broken-repo") {
		t.Fatalf("failed repos must stay visible:\n%s", s)
	}
}

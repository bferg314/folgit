package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/gitinfo"
)

// switchRepo is a clone on main with a local "feature" branch that changes
// a.txt and a remote-only "remote-only" branch.
func switchRepo(t *testing.T) (*App, string) {
	t.Helper()
	t.Setenv("LocalAppData", t.TempDir())
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	gitT(t, root, "init", "-q", "--bare", "-b", "main", origin)
	gitT(t, root, "clone", "-q", origin, work)
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main")
	gitT(t, work, "add", ".")
	gitT(t, work, "commit", "-q", "-m", "init")
	gitT(t, work, "push", "-q", "origin", "main", "main:remote-only")
	gitT(t, work, "checkout", "-q", "-b", "feature")
	write("feature")
	gitT(t, work, "commit", "-q", "-am", "feature")
	gitT(t, work, "checkout", "-q", "main")
	gitT(t, work, "fetch", "-q")

	a := New(config.Default(), filepath.Join(t.TempDir(), "config.toml"), work, nil, "")
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	r := a.local.add(work, work, 0)
	r.status = &gitinfo.Status{Branch: "main"}
	a.local.refresh()
	return a, work
}

func head(t *testing.T, dir string) string {
	return strings.TrimSpace(gitT(t, dir, "branch", "--show-current"))
}

func TestSwitchToLocalBranch(t *testing.T) {
	a, work := switchRepo(t)
	_, cmd := a.Update(tea.KeyPressMsg{Code: 'S', Text: "S"})
	run(a, cmd)
	v := a.switcher
	if v == nil || v.loading || len(v.view) != 3 || !v.view[0].Current {
		t.Fatalf("expected main, feature and origin/remote-only: %+v", v)
	}
	if s := screen(a); !strings.Contains(s, "Switch branch") || !strings.Contains(s, "creates local remote-only") {
		t.Fatalf("unexpected popup:\n%s", s)
	}

	a.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	run(a, cmd)
	if got := head(t, work); got != "feature" {
		t.Fatalf("HEAD is %q, want feature", got)
	}
	if a.switcher != nil || !strings.Contains(a.toast.text, "Switched") {
		t.Fatalf("popup should close with a toast, toast=%q", a.toast.text)
	}
}

func TestSwitchFilterToRemoteBranch(t *testing.T) {
	a, work := switchRepo(t)
	_, cmd := a.Update(tea.KeyPressMsg{Code: 'S', Text: "S"})
	run(a, cmd)
	a.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, c := range "remote" {
		a.Update(tea.KeyPressMsg{Code: c, Text: string(c)})
	}
	if v := a.switcher; len(v.view) != 1 || v.cursor != 0 {
		t.Fatalf("filter should leave only origin/remote-only: %+v", v.view)
	}
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	run(a, cmd)
	if got := head(t, work); got != "remote-only" {
		t.Fatalf("HEAD is %q, want remote-only", got)
	}
	if up := strings.TrimSpace(gitT(t, work, "rev-parse", "--abbrev-ref", "@{upstream}")); up != "origin/remote-only" {
		t.Errorf("upstream is %q, want origin/remote-only", up)
	}
}

func TestSwitchFailureKeepsPopupOpen(t *testing.T) {
	a, work := switchRepo(t)
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, cmd := a.Update(tea.KeyPressMsg{Code: 'S', Text: "S"})
	run(a, cmd)
	a.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) // feature changes a.txt too
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	run(a, cmd)
	if got := head(t, work); got != "main" {
		t.Fatalf("HEAD is %q, want main", got)
	}
	if a.switcher == nil || a.switcher.err == nil || !strings.Contains(screen(a), "commit or stash") {
		t.Fatalf("the popup should stay open and show git's error:\n%s", screen(a))
	}
}

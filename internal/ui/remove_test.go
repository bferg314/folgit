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

func TestRemoveMovesRepoToTrash(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	t.Setenv("LocalAppData", filepath.Join(tmp, "cache"))
	root := filepath.Join(tmp, "code")
	repo := filepath.Join(root, "doomed")
	gitT(t, tmp, "init", "-q", repo)
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "init")

	cfg := config.Default()
	cfg.SetPinned(repo, true)
	a := New(cfg, filepath.Join(tmp, "config.toml"), root, nil, "")
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.local.add(repo, root, 0).status = &gitinfo.Status{Branch: "main"}
	a.local.refresh()

	_, cmd := a.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	run(a, cmd)
	v := a.remove
	if v == nil || v.loading || v.check.Blocked != "" {
		t.Fatalf("popup should be ready: %+v", v)
	}
	s := screen(a)
	for _, want := range []string{"Delete folder", "No remotes", "goes to the trash", "Type doomed to confirm"} {
		if !strings.Contains(s, want) {
			t.Fatalf("popup missing %q:\n%s", want, s)
		}
	}

	// enter does nothing until the name is typed exactly.
	typeText(a, "doome")
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	run(a, cmd)
	if a.remove == nil || v.removing {
		t.Fatal("a partial name must not delete")
	}
	if _, err := os.Stat(repo); err != nil {
		t.Fatalf("repo should still exist: %v", err)
	}

	typeText(a, "d")
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	run(a, cmd)
	if a.remove != nil {
		t.Fatalf("popup should close after deleting, err=%v", v.err)
	}
	if _, err := os.Stat(repo); !os.IsNotExist(err) {
		t.Fatalf("repo should be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "data", "Trash", "files", "doomed", ".git")); err != nil {
		t.Fatalf("repo should be in the trash: %v", err)
	}
	if a.local.rows[repo] != nil || len(a.cfg.Pinned) != 0 {
		t.Fatalf("row and pin should be dropped: rows=%v pinned=%v", a.local.rows, a.cfg.Pinned)
	}
	if !strings.Contains(a.toast.text, "Moved doomed to the trash") {
		t.Fatalf("toast = %q", a.toast.text)
	}
}

func TestRemoveRechecksBeforeDeleting(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	root := filepath.Join(tmp, "code")
	repo := filepath.Join(root, "busy")
	gitT(t, tmp, "init", "-q", repo)

	a := New(config.Default(), filepath.Join(tmp, "config.toml"), root, nil, "")
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.local.add(repo, root, 0).status = &gitinfo.Status{}
	a.local.refresh()
	_, cmd := a.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	run(a, cmd)

	// A file appears after the check was shown.
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	typeText(a, "busy")
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	run(a, cmd)

	if _, err := os.Stat(filepath.Join(repo, "new.txt")); err != nil {
		t.Fatalf("nothing should be deleted when the check changes: %v", err)
	}
	if a.remove == nil || !a.remove.changed || a.remove.check.Changes != 1 || a.remove.input.Value() != "" {
		t.Fatalf("popup should show the new check and ask again: %+v", a.remove)
	}
	if s := screen(a); !strings.Contains(s, "Something changed") || !strings.Contains(s, "1 uncommitted change") {
		t.Fatalf("popup should explain:\n%s", s)
	}
}

func TestRemoveBlocksScanRoot(t *testing.T) {
	tmp := t.TempDir()
	gitT(t, tmp, "init", "-q")
	a := New(config.Default(), filepath.Join(tmp, "config.toml"), tmp, nil, "")
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.local.add(tmp, tmp, 0).status = &gitinfo.Status{}
	a.local.refresh()
	_, cmd := a.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	run(a, cmd)

	if s := screen(a); !strings.Contains(s, "Can't delete this folder: it's the folder folgit is scanning") || strings.Contains(s, "to confirm") {
		t.Fatalf("scan root should be blocked:\n%s", s)
	}
	typeText(a, filepath.Base(tmp))
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.remove != nil {
		t.Fatal("enter should just close a blocked popup")
	}
	if _, err := os.Stat(filepath.Join(tmp, ".git")); err != nil {
		t.Fatal(err)
	}
}

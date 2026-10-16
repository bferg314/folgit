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

// checksApp is a repo with an HTTPS GitHub remote and a file committed
// with CRLF, in an app that prefers SSH.
func checksApp(t *testing.T) (*App, string) {
	t.Helper()
	t.Setenv("LocalAppData", t.TempDir())
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "main")
	gitT(t, dir, "config", "core.autocrlf", "false")
	gitT(t, dir, "remote", "add", "origin", "https://github.com/o/r.git")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "add", ".")
	gitT(t, dir, "commit", "-q", "-m", "init")

	a := New(config.Default(), filepath.Join(t.TempDir(), "config.toml"), dir, nil, "")
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	a.protocol, a.protocolFrom = "ssh", "gh"
	r := a.local.add(dir, dir, 0)
	r.status = &gitinfo.Status{Branch: "main", Remotes: []string{"https://github.com/o/r.git"}}
	a.local.refresh()
	return a, dir
}

func TestChecksFlagFromScan(t *testing.T) {
	a, _ := checksApp(t)
	r := a.local.selected()
	if !a.failing(r) || !strings.Contains(a.statusCell(r), "⚑") {
		t.Fatal("an HTTPS remote should be flagged before details load")
	}
	a.protocol = "https"
	if a.failing(r) {
		t.Fatal("HTTPS remotes are fine when clones use HTTPS")
	}
}

func TestChecksPopupFixes(t *testing.T) {
	a, dir := checksApp(t)
	_, cmd := a.Update(key("C"))
	run(a, cmd)
	s := screen(a)
	for _, want := range []string{"Repo checks", "origin uses HTTPS", "1 file committed with CRLF", "CHECKS"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}

	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	run(a, cmd)
	if got := strings.TrimSpace(gitT(t, dir, "remote", "get-url", "origin")); got != "git@github.com:o/r.git" {
		t.Fatalf("origin is %q", got)
	}

	a.Update(key("j"))
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	run(a, cmd)
	if a.checks == nil || a.checks.err != nil {
		t.Fatalf("popup should stay open without an error: %+v", a.checks)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(b) != "one\n" {
		t.Fatalf("a.txt is %q", b)
	}
	if !strings.Contains(a.toast.text, "added .gitattributes") {
		t.Fatalf("toast is %q", a.toast.text)
	}
	if a.failing(a.local.selected()) {
		t.Fatalf("checks still fail:\n%s", screen(a))
	}
	a.Update(key("esc"))
	if a.checks != nil {
		t.Fatal("esc should close the popup")
	}
}

func TestChecksRefuseUncommitted(t *testing.T) {
	a, dir := checksApp(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, cmd := a.Update(key("C"))
	run(a, cmd)
	a.Update(key("j"))
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	run(a, cmd)
	if a.checks == nil || a.checks.err == nil || !strings.Contains(screen(a), "commit or stash") {
		t.Fatalf("expected a refusal:\n%s", screen(a))
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(b) != "two\r\n" {
		t.Fatalf("a.txt was changed: %q", b)
	}
}

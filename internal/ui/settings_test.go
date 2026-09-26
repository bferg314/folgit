package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/config"
)

// Opening Settings picks up a tool installed while folgit was running.
func TestSettingsTabEnablesNewlyInstalledTool(t *testing.T) {
	t.Setenv("LocalAppData", t.TempDir())
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("PATHEXT", ".EXE")

	cfg := &config.Config{Version: 1, Tools: []config.Tool{
		{Name: "lazygit", Cmd: "lazygit", Key: "l", Mode: config.ModeTerminal, AutoDisabled: true},
	}}
	a := New(cfg, filepath.Join(t.TempDir(), "config.toml"), t.TempDir(), nil, "")
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 24})

	name := "lazygit"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	a.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	if !cfg.Tools[0].Enabled || !strings.Contains(a.toast.text, "lazygit") {
		t.Fatalf("lazygit should be enabled with a toast; enabled=%v toast=%q", cfg.Tools[0].Enabled, a.toast.text)
	}
	if data, err := os.ReadFile(a.cfgPath); err != nil || !strings.Contains(string(data), "enabled = true") {
		t.Fatalf("config not saved: %v\n%s", err, data)
	}
}

func TestSettingsDefaultFolder(t *testing.T) {
	t.Setenv("LocalAppData", t.TempDir())
	root := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.Default()
	a := New(cfg, cfgPath, root, nil, "")
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	a.Update(tea.KeyPressMsg{Code: '3', Text: "3"})

	s := screen(a)
	if !strings.Contains(s, "Default folder") || !strings.Contains(s, "not set: opens where you launch folgit") {
		t.Fatalf("default folder row missing:\n%s", s)
	}

	// The row is first; enter sets it to the folder being viewed.
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cfg.DefaultDir != tildePath(root) || !strings.Contains(a.toast.text, "by default") {
		t.Fatalf("enter should set default_dir: %q toast=%q", cfg.DefaultDir, a.toast.text)
	}
	if data, _ := os.ReadFile(cfgPath); !strings.Contains(string(data), "default_dir") {
		t.Fatalf("not saved:\n%s", data)
	}
	if config.ExpandPath(cfg.DefaultDir) != filepath.Clean(root) {
		t.Fatalf("saved value doesn't expand back to the folder: %q", cfg.DefaultDir)
	}

	// A default that no longer exists is flagged.
	cfg.DefaultDir = filepath.Join(root, "gone")
	if !strings.Contains(screen(a), "folder not found") {
		t.Fatalf("missing folder should be flagged:\n%s", screen(a))
	}

	a.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cfg.DefaultDir != "" || !strings.Contains(a.toast.text, "cleared") {
		t.Fatalf("x should clear: %q toast=%q", cfg.DefaultDir, a.toast.text)
	}
}

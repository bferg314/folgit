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

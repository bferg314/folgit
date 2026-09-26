package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMigrateAntigravity(t *testing.T) {
	fakePath(t, "agy", "antigravity-ide")
	c := &Config{Version: 1, Tools: []Tool{
		{Name: "Claude Code", Cmd: "claude", Key: "c", Mode: ModeTerminal, Enabled: true},
		{Name: "Gemini", Cmd: "gemini", Key: "m", Mode: ModeTerminal, AutoDisabled: true},
		{Name: "Antigravity", Cmd: "agy", Args: []string{"{path}"}, Key: "a", Mode: ModeDetach, Enabled: true},
		{Name: "Custom agy", Cmd: "agy", Args: []string{"--new-window", "{path}"}, Key: "N", Mode: ModeDetach},
		{Name: "VS Code", Cmd: "code", Args: []string{"{path}"}, Key: "o", Mode: ModeDetach},
	}}
	if !c.migrate() || c.Version != 2 {
		t.Fatalf("expected a v2 migration, version=%d", c.Version)
	}

	var names []string
	for _, tool := range c.Tools {
		names = append(names, tool.Name)
	}
	if got := strings.Join(names, ","); got != "Claude Code,Antigravity,Antigravity IDE,Custom agy,VS Code" {
		t.Fatalf("tools = %s", got)
	}
	if agy := c.Tools[1]; agy.Mode != ModeTerminal || len(agy.Args) != 0 || !agy.Enabled {
		t.Errorf("agy should run in the terminal: %+v", agy)
	}
	if ide := c.Tools[2]; ide.Mode != ModeDetach || !ide.Enabled || ide.Key != "A" {
		t.Errorf("IDE should be added, detached and enabled (installed): %+v", ide)
	}
	if custom := c.Tools[3]; custom.Mode != ModeDetach {
		t.Errorf("user-customised agy entry was changed: %+v", custom)
	}

	// Running it again changes nothing.
	if c.migrate() || len(c.Tools) != 5 {
		t.Fatalf("second migrate changed things: %+v", c.Tools)
	}
}

// A Gemini entry the user switched on is kept.
func TestMigrateKeepsEnabledGemini(t *testing.T) {
	fakePath(t)
	c := &Config{Version: 1, Tools: []Tool{{Name: "Gemini", Cmd: "gemini", Key: "m", Mode: ModeTerminal, Enabled: true}}}
	c.migrate()
	if c.Tools[0].Name != "Gemini" || c.Tools[1].Name != "Antigravity IDE" || c.Tools[1].Enabled || !c.Tools[1].AutoDisabled {
		t.Fatalf("got %+v", c.Tools)
	}
}

// fakePath makes a directory with fake executables for names and puts only
// that directory on PATH.
func fakePath(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	fakeInstall(t, dir, names...)
	t.Setenv("PATH", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".EXE")
	}
	return dir
}

func TestEnableInstalled(t *testing.T) {
	dir := fakePath(t, "claude")
	c := &Config{Version: configVersion, Tools: []Tool{
		{Name: "Claude Code", Cmd: "claude"},
		{Name: "lazygit", Cmd: "lazygit"},
		{Name: "Codex", Cmd: "codex"},
	}}
	c.DetectTools()
	if !c.Tools[0].Enabled || c.Tools[1].Enabled || !c.Tools[1].AutoDisabled {
		t.Fatalf("detect: %+v", c.Tools)
	}

	// The user turns Codex off by hand (clears AutoDisabled), then installs
	// lazygit and codex.
	c.Tools[2].AutoDisabled = false
	fakeInstall(t, dir, "lazygit", "codex")

	got := c.EnableInstalled()
	if len(got) != 1 || got[0] != "lazygit" || !c.Tools[1].Enabled || c.Tools[1].AutoDisabled {
		t.Fatalf("lazygit should be enabled, got %v %+v", got, c.Tools[1])
	}
	if c.Tools[2].Enabled {
		t.Fatal("a tool the user turned off must stay off")
	}
	if again := c.EnableInstalled(); len(again) != 0 {
		t.Fatalf("second call should be a no-op, got %v", again)
	}
}

func fakeInstall(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		file := filepath.Join(dir, n)
		if runtime.GOOS == "windows" {
			file += ".exe"
		}
		if err := os.WriteFile(file, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// A config written before versioning (like the one that hid lazygit)
// picks up tools installed since it was created.
func TestLoadUpgradesUnversionedConfig(t *testing.T) {
	fakePath(t, "claude", "lazygit")
	path := filepath.Join(t.TempDir(), "config.toml")
	old := `max_depth = 4

[[tools]]
  name = "Claude Code"
  cmd = "claude"
  key = "c"
  mode = "terminal"
  enabled = true

[[tools]]
  name = "lazygit"
  cmd = "lazygit"
  key = "l"
  mode = "terminal"
  enabled = false

[[tools]]
  name = "Codex"
  cmd = "codex"
  key = "x"
  mode = "terminal"
  enabled = false
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != configVersion || !c.Tools[1].Enabled {
		t.Fatalf("lazygit should now be enabled: version=%d %+v", c.Version, c.Tools[1])
	}
	if c.Tools[2].Enabled || !c.Tools[2].AutoDisabled {
		t.Fatalf("codex isn't installed: it should stay off but be marked auto: %+v", c.Tools[2])
	}

	// The upgrade was saved, and loading again changes nothing.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "version = 2") || !strings.Contains(string(data), "auto_disabled = true") {
		t.Fatalf("upgrade not saved:\n%s", data)
	}
	if c2, err := Load(path); err != nil || !c2.Tools[1].Enabled || c2.Tools[2].Enabled {
		t.Fatalf("reload changed things: %v %+v", err, c2.Tools)
	}
}

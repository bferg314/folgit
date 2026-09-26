package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMigrateAgy(t *testing.T) {
	c := &Config{Version: configVersion, Tools: []Tool{
		{Name: "Antigravity", Cmd: "agy", Args: []string{"{path}"}, Key: "a", Mode: ModeDetach},
		{Name: "Custom agy", Cmd: "agy", Args: []string{"--new-window", "{path}"}, Key: "A", Mode: ModeDetach},
	}}
	changed := c.migrate()

	if runtime.GOOS != "linux" {
		if changed || c.Tools[0].Mode != ModeDetach {
			t.Fatalf("migrate changed agy on %s: %+v", runtime.GOOS, c.Tools[0])
		}
		return
	}
	if !changed || c.Tools[0].Mode != ModeTerminal || len(c.Tools[0].Args) != 0 {
		t.Fatalf("old default not migrated: %+v", c.Tools[0])
	}
	if c.Tools[1].Mode != ModeDetach {
		t.Fatalf("user-customised tool was changed: %+v", c.Tools[1])
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
	if !strings.Contains(string(data), "version = 1") || !strings.Contains(string(data), "auto_disabled = true") {
		t.Fatalf("upgrade not saved:\n%s", data)
	}
	if c2, err := Load(path); err != nil || !c2.Tools[1].Enabled || c2.Tools[2].Enabled {
		t.Fatalf("reload changed things: %v %+v", err, c2.Tools)
	}
}

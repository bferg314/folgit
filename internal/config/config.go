// Package config loads and saves folgit's TOML configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

// Tool launch modes.
const (
	// ModeTerminal suspends the TUI and hands the terminal to the tool
	// (claude, codex, lazygit, ...). folgit resumes when it exits.
	ModeTerminal = "terminal"
	// ModeDetach starts the tool in the background and returns immediately
	// (VS Code, file managers, other GUI apps).
	ModeDetach = "detach"
)

// Tool is an external program that can be launched inside a repository.
type Tool struct {
	Name    string   `toml:"name"`
	Cmd     string   `toml:"cmd"`
	Args    []string `toml:"args"` // "{path}" is replaced with the repo path
	Key     string   `toml:"key"`
	Mode    string   `toml:"mode"`
	Enabled bool     `toml:"enabled"`
	// AutoDisabled marks a tool folgit switched off because its command
	// wasn't installed. It is switched back on once the command appears.
	// Toggling a tool by hand clears it, so user choices stick.
	AutoDisabled bool `toml:"auto_disabled,omitempty"`
}

// GitHub controls which remote repositories are listed.
type GitHub struct {
	IncludeOrgs     bool `toml:"include_orgs"`
	IncludeStarred  bool `toml:"include_starred"`
	IncludeArchived bool `toml:"include_archived"`
	IncludeForks    bool `toml:"include_forks"`
	// Protocol is "ssh" or "https". Empty means use `gh config get git_protocol`.
	Protocol string `toml:"protocol"`
}

// Config is the full on-disk configuration.
type Config struct {
	// Version is the config format; see migrate.
	Version    int      `toml:"version"`
	MaxDepth   int      `toml:"max_depth"`
	ScanHidden bool     `toml:"scan_hidden"`
	Ignore     []string `toml:"ignore"`
	// CloneLayout decides where remote repos are cloned, relative to the
	// scanned directory. Supports {host}, {owner} and {repo}.
	CloneLayout string `toml:"clone_layout"`
	GitHub      GitHub `toml:"github"`
	Tools       []Tool `toml:"tools"`
}

// Path returns the location of the config file.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "folgit", "config.toml"), nil
}

// Load reads the config at path. If it does not exist, defaults are created,
// installed tools are enabled, and the result is written to disk.
func Load(path string) (*Config, error) {
	cfg := Default()
	cfg.Version = 0 // files from before versioning have no version key
	_, err := toml.DecodeFile(path, cfg)
	if errors.Is(err, fs.ErrNotExist) {
		cfg.Version = configVersion
		cfg.DetectTools()
		return cfg, cfg.Save(path)
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = Default().MaxDepth
	}
	migrated := cfg.migrate()
	if enabled := cfg.EnableInstalled(); migrated || len(enabled) > 0 {
		if err := cfg.Save(path); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// configVersion is the current config format.
//
//	0: no version key; tool detection ran only when the file was created
//	1: tools folgit disabled for being missing carry auto_disabled
const configVersion = 1

// migrate upgrades older configs. It only touches values that still match
// an old default, so user edits are left alone.
func (c *Config) migrate() bool {
	changed := false
	if c.Version < 1 {
		// Version 0 couldn't tell "not installed at setup" from "turned off
		// by the user". Treat every disabled tool as not-installed-at-setup,
		// since that was the only way folgit itself disabled tools.
		for i := range c.Tools {
			if !c.Tools[i].Enabled {
				c.Tools[i].AutoDisabled = true
			}
		}
		c.Version = 1
		changed = true
	}
	want := agyTool()
	for i := range c.Tools {
		t := &c.Tools[i]
		// Before v0.3 agy defaulted to detach mode everywhere, but on Linux
		// it is a terminal program.
		if t.Cmd == "agy" && t.Mode == ModeDetach && len(t.Args) == 1 && t.Args[0] == "{path}" && want.Mode != ModeDetach {
			t.Mode, t.Args = want.Mode, want.Args
			changed = true
		}
	}
	return changed
}

// Save writes the config to path, creating parent directories as needed.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("# folgit configuration\n# Tool args: \"{path}\" is replaced with the repository path.\n# Tool modes: \"terminal\" (suspends folgit) or \"detach\" (runs in background).\n\n")
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// DetectTools enables every configured tool whose command is on PATH and
// marks the rest as auto-disabled, so they switch on once installed.
func (c *Config) DetectTools() {
	for i := range c.Tools {
		ok := Available(c.Tools[i].Cmd)
		c.Tools[i].Enabled, c.Tools[i].AutoDisabled = ok, !ok
	}
}

// EnableInstalled switches on auto-disabled tools whose command is now on
// PATH and returns their names. Tools the user turned off are untouched.
func (c *Config) EnableInstalled() []string {
	var names []string
	for i := range c.Tools {
		t := &c.Tools[i]
		if t.AutoDisabled && !t.Enabled && Available(t.Cmd) {
			t.Enabled, t.AutoDisabled = true, false
			names = append(names, t.Name)
		}
	}
	return names
}

// Default returns the built-in configuration.
func Default() *Config {
	return &Config{
		Version:  configVersion,
		MaxDepth: 4,
		Ignore: []string{
			"node_modules", "vendor", "target", "dist", "build", "out",
			".venv", "venv", "__pycache__", ".next", ".cache", "bin", "obj",
		},
		CloneLayout: "{repo}",
		GitHub: GitHub{
			IncludeOrgs:  true,
			IncludeForks: true,
		},
		Tools: []Tool{
			{Name: "Claude Code", Cmd: "claude", Key: "c", Mode: ModeTerminal},
			{Name: "Codex", Cmd: "codex", Key: "x", Mode: ModeTerminal},
			{Name: "Gemini", Cmd: "gemini", Key: "m", Mode: ModeTerminal},
			agyTool(),
			{Name: "VS Code", Cmd: "code", Args: []string{"{path}"}, Key: "o", Mode: ModeDetach},
			{Name: "Cursor", Cmd: "cursor", Args: []string{"{path}"}, Key: "u", Mode: ModeDetach},
			{Name: "lazygit", Cmd: "lazygit", Key: "l", Mode: ModeTerminal},
			fileManager(),
		},
	}
}

// agyTool is Antigravity: a desktop launcher on Windows and macOS, but a
// terminal program on Linux, where it must keep the TTY.
func agyTool() Tool {
	t := Tool{Name: "Antigravity", Cmd: "agy", Key: "a"}
	if runtime.GOOS == "linux" {
		t.Mode = ModeTerminal
	} else {
		t.Mode, t.Args = ModeDetach, []string{"{path}"}
	}
	return t
}

func fileManager() Tool {
	t := Tool{Name: "File manager", Args: []string{"{path}"}, Key: "e", Mode: ModeDetach}
	switch runtime.GOOS {
	case "windows":
		t.Cmd = "explorer"
	case "darwin":
		t.Cmd = "open"
	default:
		t.Cmd = "xdg-open"
	}
	return t
}

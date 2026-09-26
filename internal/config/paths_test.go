package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	t.Setenv("FOLGIT_TEST_DIR", filepath.Join(home, "x"))
	cases := map[string]string{
		"":                            "",
		"~":                           home,
		"~/code":                      filepath.Join(home, "code"),
		"$FOLGIT_TEST_DIR/y":          filepath.Join(home, "x", "y"),
		"${FOLGIT_TEST_DIR}/y":        filepath.Join(home, "x", "y"),
		"%FOLGIT_TEST_DIR%/y":         filepath.Join(home, "x", "y"),
		"%FOLGIT_NOT_SET_XYZ%/y":      filepath.Clean("%FOLGIT_NOT_SET_XYZ%/y"),
		"  ~/spaced  ":                filepath.Join(home, "spaced"),
		filepath.Join("a", "..", "b"): "b",
	}
	if runtime.GOOS == "windows" {
		cases[`~\code`] = filepath.Join(home, "code")
	}
	for in, want := range cases {
		if got := ExpandPath(in); got != want {
			t.Errorf("ExpandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveDir(t *testing.T) {
	def := t.TempDir()
	c := &Config{DefaultDir: def}

	if dir, notice := c.ResolveDir("somewhere"); dir != "somewhere" || notice != "" {
		t.Errorf("an argument must win: %q %q", dir, notice)
	}
	if dir, notice := c.ResolveDir(""); dir != def || notice != "" {
		t.Errorf("default_dir should be used: %q %q", dir, notice)
	}
	if dir, notice := (&Config{}).ResolveDir(""); dir != "." || notice != "" {
		t.Errorf("no default: current dir: %q %q", dir, notice)
	}

	missing := &Config{DefaultDir: filepath.Join(def, "gone")}
	if dir, notice := missing.ResolveDir(""); dir != "." || !strings.Contains(notice, "not found") {
		t.Errorf("missing default_dir should fall back with a notice: %q %q", dir, notice)
	}
	file := filepath.Join(def, "file.txt")
	os.WriteFile(file, nil, 0o644)
	if dir, _ := (&Config{DefaultDir: file}).ResolveDir(""); dir != "." {
		t.Errorf("a file isn't a folder: %q", dir)
	}
}

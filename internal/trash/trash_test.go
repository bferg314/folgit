//go:build linux

package trash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMoveFreedesktop(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	src := filepath.Join(tmp, "my repo")
	if err := os.MkdirAll(filepath.Join(src, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got, want := Dir(src), filepath.Join(tmp, "data", "Trash"); got != want {
		t.Fatalf("Dir = %q, want %q", got, want)
	}
	dest, err := Move(src)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(tmp, "data", "Trash", "files", "my repo"); dest != want {
		t.Fatalf("dest = %q, want %q", dest, want)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		t.Fatalf("contents should have moved: %v", err)
	}
	info, err := os.ReadFile(filepath.Join(tmp, "data", "Trash", "info", "my repo.trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(info), "Path="+filepath.ToSlash(tmp)+"/my%20repo\n") || !strings.Contains(string(info), "DeletionDate=") {
		t.Fatalf("bad trashinfo:\n%s", info)
	}

	// A second folder with the same name gets a numbered slot.
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	dest, err = Move(src)
	if err != nil || filepath.Base(dest) != "my repo.2" {
		t.Fatalf("second move: %q, %v", dest, err)
	}
}

func TestDirOtherFilesystem(t *testing.T) {
	// /proc is never on the same filesystem as a temp dir.
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if d := Dir("/proc/self"); d != "" {
		t.Fatalf("Dir across filesystems = %q, want \"\"", d)
	}
}

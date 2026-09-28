package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSetPinnedAndHidden(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	repo := filepath.Join(home, "code", "folgit")
	c := &Config{}

	c.SetPinned(repo, true)
	if len(c.Pinned) != 1 || c.Pinned[0] != "~/code/folgit" {
		t.Fatalf("pinned paths are stored under ~ with forward slashes: %q", c.Pinned)
	}
	c.SetPinned(repo, true)
	if len(c.Pinned) != 1 {
		t.Errorf("pinning twice must not duplicate: %q", c.Pinned)
	}
	if pinned, _ := c.RepoMarks(); !pinned[RepoKey(repo)] {
		t.Errorf("RepoMarks should report the pin: %v", pinned)
	}

	c.SetHidden(repo, true)
	if len(c.Pinned) != 0 || len(c.Hidden) != 1 {
		t.Errorf("hiding a pinned repo unpins it: pinned %q hidden %q", c.Pinned, c.Hidden)
	}
	c.SetPinned(repo, true)
	if len(c.Pinned) != 1 || len(c.Hidden) != 0 {
		t.Errorf("pinning a hidden repo unhides it: pinned %q hidden %q", c.Pinned, c.Hidden)
	}
	c.SetPinned(repo, false)
	if len(c.Pinned) != 0 {
		t.Errorf("unpin: %q", c.Pinned)
	}

	outside := filepath.Join(filepath.VolumeName(home)+string(filepath.Separator), "elsewhere", "repo")
	if !strings.HasPrefix(outside, home) {
		c.SetHidden(outside, true)
		if c.Hidden[0] != filepath.ToSlash(outside) {
			t.Errorf("paths outside home are stored whole: %q", c.Hidden)
		}
	}
}

func TestRepoKey(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if RepoKey("~/code/x") != RepoKey(filepath.Join(home, "code", "x")) {
		t.Error("~ and the full path should be the same repo")
	}
	if runtime.GOOS == "windows" && RepoKey(`C:\Code\X`) != RepoKey(`c:\code\x`) {
		t.Error("Windows paths ignore case")
	}
}

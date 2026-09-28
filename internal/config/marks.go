package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RepoKey normalizes a repo path so the same repo always compares equal:
// ~ and environment variables are expanded, separators are made forward
// slashes and, on Windows, case is ignored.
func RepoKey(p string) string {
	p = filepath.ToSlash(ExpandPath(p))
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// storedPath is how a repo path is written to the config: the home
// directory as ~ and forward slashes, so the file reads the same everywhere.
func storedPath(p string) string {
	p = filepath.Clean(p)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			p = "~" + string(filepath.Separator) + rel
		}
	}
	return filepath.ToSlash(p)
}

// RepoMarks returns the keys (see RepoKey) of the pinned and hidden repos.
func (c *Config) RepoMarks() (pinned, hidden map[string]bool) {
	return keySet(c.Pinned), keySet(c.Hidden)
}

// SetPinned pins or unpins the repo at path. Pinning a hidden repo unhides it.
func (c *Config) SetPinned(path string, on bool) {
	c.Pinned = setMark(c.Pinned, path, on)
	if on {
		c.Hidden = setMark(c.Hidden, path, false)
	}
}

// SetHidden hides or unhides the repo at path. Hiding a pinned repo unpins it.
func (c *Config) SetHidden(path string, on bool) {
	c.Hidden = setMark(c.Hidden, path, on)
	if on {
		c.Pinned = setMark(c.Pinned, path, false)
	}
}

func keySet(paths []string) map[string]bool {
	m := make(map[string]bool, len(paths))
	for _, p := range paths {
		m[RepoKey(p)] = true
	}
	return m
}

// setMark adds path to list, or removes every entry for it.
func setMark(list []string, path string, on bool) []string {
	key := RepoKey(path)
	out := list[:0:0]
	for _, p := range list {
		if RepoKey(p) != key {
			out = append(out, p)
		}
	}
	if on {
		out = append(out, storedPath(path))
	}
	return out
}

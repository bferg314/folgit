package gitinfo

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Details is the extra information shown in the detail pane. It is loaded
// on demand for the selected repo only.
type Details struct {
	Commits  []Commit
	Branches []Branch
	Remotes  []Remote
	Readme   []string // first lines of the README, if any
}

type Commit struct {
	Hash    string
	Subject string
	Author  string
	When    time.Time
}

type Branch struct {
	Name     string
	Upstream string
	Track    string // e.g. "ahead 1, behind 2" or "gone"
	When     time.Time
}

type Remote struct {
	Name string
	URL  string
}

const sep = "\x1f"

// GetDetails loads commits, branches, remotes and a README preview. Parts
// that fail (e.g. no commits yet) are simply left empty.
func GetDetails(ctx context.Context, dir string, commits, branches, readmeLines int) Details {
	var d Details

	if out, err := Git(ctx, dir, "log", "-n", strconv.Itoa(commits), "--format=%h"+sep+"%s"+sep+"%an"+sep+"%ct"); err == nil {
		for _, f := range fields(out, 4) {
			d.Commits = append(d.Commits, Commit{Hash: f[0], Subject: f[1], Author: f[2], When: unix(f[3])})
		}
	}

	if out, err := Git(ctx, dir, "for-each-ref", "--sort=-committerdate", "--count="+strconv.Itoa(branches),
		"--format=%(refname:short)"+sep+"%(upstream:short)"+sep+"%(upstream:track,nobracket)"+sep+"%(committerdate:unix)",
		"refs/heads"); err == nil {
		for _, f := range fields(out, 4) {
			d.Branches = append(d.Branches, Branch{Name: f[0], Upstream: f[1], Track: f[2], When: unix(f[3])})
		}
	}

	if out, err := Git(ctx, dir, "remote", "-v"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			f := strings.Fields(line)
			if len(f) == 3 && f[2] == "(fetch)" {
				d.Remotes = append(d.Remotes, Remote{Name: f[0], URL: f[1]})
			}
		}
	}

	d.Readme = readme(dir, readmeLines)
	return d
}

func fields(out []byte, n int) [][]string {
	var rows [][]string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if f := strings.SplitN(line, sep, n); len(f) == n {
			rows = append(rows, f)
		}
	}
	return rows
}

func unix(s string) time.Time {
	ts, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}

// readme returns up to n non-noise lines from the repo's README.
func readme(dir string, n int) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var name string
	for _, e := range entries {
		lower := strings.ToLower(e.Name())
		if !e.IsDir() && (lower == "readme" || strings.HasPrefix(lower, "readme.")) {
			name = e.Name()
			break
		}
	}
	if name == "" {
		return nil
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(io.LimitReader(f, 16<<10))
	blank := true
	for sc.Scan() && len(lines) < n {
		line := strings.TrimRight(sc.Text(), " \t\r")
		t := strings.TrimSpace(line)
		// Skip badges, HTML and code fences: they read as noise in a preview.
		if strings.HasPrefix(t, "[![") || strings.HasPrefix(t, "<") || strings.HasPrefix(t, "```") {
			continue
		}
		if t == "" {
			if !blank {
				lines = append(lines, "")
			}
			blank = true
			continue
		}
		blank = false
		lines = append(lines, line)
	}
	return lines
}

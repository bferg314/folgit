package gitinfo

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Removal is what deleting a repository's folder would lose, so the user
// can decide with the facts in front of them.
type Removal struct {
	// Blocked says why the folder must not be deleted at all; "" if it may.
	Blocked string

	Changes   int  // uncommitted changes, including untracked files
	Stashes   int  // stash entries, which never leave the machine
	Unpushed  int  // commits on local branches, tags or HEAD not on any remote
	NoRemotes bool // nothing has been pushed anywhere
	// Ignored lists ignored paths (directories end in /), such as .env or
	// node_modules/. git never pushes them.
	Ignored []string
	// Nested lists other repositories inside the folder, relative to it.
	// Their own changes aren't checked.
	Nested []string
}

// Risky reports whether deleting would lose work that exists nowhere else.
func (r Removal) Risky() bool {
	return r.Changes > 0 || r.Stashes > 0 || r.Unpushed > 0 || r.NoRemotes || len(r.Nested) > 0
}

// SameRisks reports whether r and o describe the same risks, ignoring the
// list of ignored files (build output churns constantly).
func (r Removal) SameRisks(o Removal) bool {
	return r.Blocked == o.Blocked && r.Changes == o.Changes && r.Stashes == o.Stashes &&
		r.Unpushed == o.Unpushed && r.NoRemotes == o.NoRemotes &&
		strings.Join(r.Nested, "\x00") == strings.Join(o.Nested, "\x00")
}

// CheckRemoval inspects the repository at dir, found by scanning root, and
// reports what deleting its folder would lose. Folders that are the scan
// root, outside it, the home directory, symlinks, linked worktrees,
// submodules, or that have linked worktrees of their own are blocked.
func CheckRemoval(ctx context.Context, root, dir string) Removal {
	var r Removal
	if why := blocked(ctx, root, dir); why != "" {
		r.Blocked = why
		return r
	}

	out, err := Git(ctx, dir, "status", "--porcelain=v2", "--branch", "--show-stash")
	if err != nil {
		r.Blocked = "git can't read it: " + err.Error()
		return r
	}
	var s Status
	parseStatus(out, &s)
	r.Changes, r.Stashes = s.Changes(), s.Stashes

	remotes, err := Git(ctx, dir, "remote")
	if err != nil {
		r.Blocked = "couldn't list its remotes: " + err.Error()
		return r
	}
	r.NoRemotes = strings.TrimSpace(string(remotes)) == ""

	// HEAD covers a detached checkout; it fails in a repo with no commits,
	// so fall back to the refs alone. If both fail the count is unknown,
	// and an unknown count isn't safe to delete on.
	counted := false
	for _, args := range [][]string{
		{"rev-list", "--count", "HEAD", "--branches", "--tags", "--not", "--remotes"},
		{"rev-list", "--count", "--branches", "--tags", "--not", "--remotes"},
	} {
		if out, err := Git(ctx, dir, args...); err == nil {
			r.Unpushed, err = strconv.Atoi(strings.TrimSpace(string(out)))
			counted = err == nil
			break
		}
	}
	if !counted {
		r.Blocked = "couldn't count its unpushed commits"
		return r
	}

	if out, err := Git(ctx, dir, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z"); err == nil {
		for _, p := range strings.Split(string(out), "\x00") {
			if p != "" {
				r.Ignored = append(r.Ignored, p)
			}
		}
	}

	r.Nested = nestedRepos(ctx, dir)
	if ctx.Err() != nil {
		// A partial walk could have missed a nested repo.
		r.Blocked = "the check didn't finish in time"
	}
	return r
}

// blocked returns why dir must never be deleted, or "".
func blocked(ctx context.Context, root, dir string) string {
	if !filepath.IsAbs(dir) {
		return "its path isn't absolute"
	}
	dir, root = filepath.Clean(dir), filepath.Clean(root)
	rel, err := filepath.Rel(root, dir)
	switch {
	case err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel):
		return "it's outside the scanned folder"
	case rel == ".":
		return "it's the folder folgit is scanning"
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == dir {
		return "it's your home directory"
	}

	fi, err := os.Lstat(dir)
	switch {
	case err != nil:
		return err.Error()
	case fi.Mode()&fs.ModeSymlink != 0:
		return "it's a symlink"
	case !fi.IsDir():
		return "it isn't a directory"
	}
	gi, err := os.Lstat(filepath.Join(dir, ".git"))
	switch {
	case err != nil:
		return "it has no .git directory"
	case !gi.IsDir():
		// A .git file means a linked worktree or a submodule, whose real
		// repository lives elsewhere and would be left pointing at nothing.
		return "it's a linked worktree or submodule: use git worktree remove or git submodule deinit"
	}

	if out, err := Git(ctx, dir, "worktree", "list", "--porcelain"); err == nil {
		if n := strings.Count(string(out), "worktree "); n > 1 {
			return strconv.Itoa(n-1) + " linked worktree(s) depend on it: remove them first"
		}
	}
	return ""
}

// nestedRepos finds repositories (or submodules) inside dir, other than
// dir itself. Symlinks aren't followed, matching the scan.
func nestedRepos(ctx context.Context, dir string) []string {
	var found []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil
		}
		if d.Name() != ".git" {
			return nil
		}
		parent := filepath.Dir(p)
		if parent != dir {
			rel, _ := filepath.Rel(dir, parent)
			found = append(found, filepath.ToSlash(rel))
		}
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

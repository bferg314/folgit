package gitinfo

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// StaleBranch is a local branch that is probably safe to delete.
type StaleBranch struct {
	Name     string
	Hash     string // short hash, so a deleted branch can be restored
	When     time.Time
	Merged   bool // its commits are all in Base
	Gone     bool // it tracked a remote branch that no longer exists
	Unmerged int  // commits not in Base (only counted when Gone && !Merged)
}

// protected branch names are never offered for deletion.
var protected = map[string]bool{"main": true, "master": true, "develop": true, "dev": true, "trunk": true}

// StaleBranches finds local branches that are merged into the default
// branch (base) or whose upstream was deleted. The current branch and
// long-lived branches (main, master, develop, ...) are never included.
// base is "" when no default branch can be found; only gone branches are
// reported then.
func StaleBranches(ctx context.Context, dir string) (branches []StaleBranch, base string, err error) {
	base = defaultBranch(ctx, dir)
	current := ""
	if out, err := Git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		current = strings.TrimSpace(string(out))
	}

	merged := map[string]bool{}
	if base != "" {
		out, err := Git(ctx, dir, "branch", "--format=%(refname:short)", "--merged", base)
		if err != nil {
			return nil, base, err
		}
		for _, name := range strings.Fields(string(out)) {
			merged[name] = true
		}
	}

	out, err := Git(ctx, dir, "for-each-ref",
		"--format=%(refname:short)"+sep+"%(upstream:track,nobracket)"+sep+"%(committerdate:unix)"+sep+"%(objectname:short)",
		"refs/heads")
	if err != nil {
		return nil, base, err
	}
	baseName := base[strings.LastIndex(base, "/")+1:]
	for _, f := range fields(out, 4) {
		name := f[0]
		if name == current || protected[name] || name == baseName {
			continue
		}
		b := StaleBranch{Name: name, Hash: f[3], When: unix(f[2]), Merged: merged[name], Gone: f[1] == "gone"}
		if !b.Merged && !b.Gone {
			continue
		}
		if b.Gone && !b.Merged && base != "" {
			if out, err := Git(ctx, dir, "rev-list", "--count", base+".."+name); err == nil {
				b.Unmerged, _ = strconv.Atoi(strings.TrimSpace(string(out)))
			}
		}
		branches = append(branches, b)
	}
	return branches, base, nil
}

// defaultBranch returns the ref other branches are merged into: origin's
// HEAD if known, else origin/main or origin/master, else a local main or
// master. It prefers the remote so branches merged on GitHub count even if
// the local main is behind.
func defaultBranch(ctx context.Context, dir string) string {
	// origin/HEAD can be stale: after the default branch is renamed on the
	// host (master -> main) it still points at a ref that's gone, so only
	// trust it if its target exists.
	if out, err := Git(ctx, dir, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		full := strings.TrimSpace(string(out))
		if _, err := Git(ctx, dir, "rev-parse", "--verify", "--quiet", full); err == nil {
			return strings.TrimPrefix(full, "refs/remotes/")
		}
	}
	for _, ref := range []string{"origin/main", "origin/master", "main", "master"} {
		full := "refs/heads/" + ref
		if strings.HasPrefix(ref, "origin/") {
			full = "refs/remotes/" + ref
		}
		if _, err := Git(ctx, dir, "rev-parse", "--verify", "--quiet", full); err == nil {
			return ref
		}
	}
	return ""
}

// DeleteBranch force-deletes a local branch. Callers must have checked it
// is merged, or have the user's explicit say-so for unmerged commits.
func DeleteBranch(ctx context.Context, dir, name string) error {
	_, err := Git(ctx, dir, "branch", "-D", name)
	return err
}

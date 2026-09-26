package gitinfo

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// cleanupFixture builds a clone of a bare origin with one branch per case:
//
//	merged-local   merged into main, never pushed          -> merged
//	merged-gone    merged, pushed, remote branch deleted   -> merged + gone
//	squashed       pushed, remote deleted, commit not in main (squash merge) -> gone, 1 unmerged
//	wip            local only, unmerged                    -> not stale
//	current        checked out, merged                     -> never offered
//	develop        merged, but long-lived                  -> never offered
func cleanupFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	ctx := context.Background()
	git := func(dir string, args ...string) {
		t.Helper()
		args = append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)
		if _, err := Git(ctx, dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	commit := func(msg string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(work, msg+".txt"), []byte(msg), 0o644); err != nil {
			t.Fatal(err)
		}
		git(work, "add", ".")
		git(work, "commit", "-q", "-m", msg)
	}

	git(root, "init", "-q", "--bare", "-b", "main", origin)
	git(root, "clone", "-q", origin, work)
	commit("init")
	git(work, "push", "-q", "origin", "main")
	git(work, "remote", "set-head", "origin", "main")

	for _, b := range []string{"merged-local", "develop", "current"} {
		git(work, "branch", b)
	}

	git(work, "checkout", "-q", "-b", "merged-gone")
	commit("feature")
	git(work, "push", "-q", "-u", "origin", "merged-gone")
	git(work, "checkout", "-q", "main")
	git(work, "merge", "-q", "--ff-only", "merged-gone")
	git(work, "push", "-q", "origin", "main", ":merged-gone")

	git(work, "checkout", "-q", "-b", "squashed")
	commit("squash-me")
	git(work, "push", "-q", "-u", "origin", "squashed")
	git(work, "push", "-q", "origin", ":squashed")

	git(work, "checkout", "-q", "-b", "wip", "main")
	commit("wip")

	git(work, "checkout", "-q", "current")
	git(work, "fetch", "-q", "--prune")
	return work
}

func TestStaleBranches(t *testing.T) {
	dir := cleanupFixture(t)
	got, base, err := StaleBranches(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if base != "origin/main" {
		t.Errorf("base = %q, want origin/main", base)
	}

	byName := map[string]StaleBranch{}
	var names []string
	for _, b := range got {
		byName[b.Name] = b
		names = append(names, b.Name)
	}
	sort.Strings(names)
	if want := []string{"merged-gone", "merged-local", "squashed"}; len(names) != 3 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("stale = %v, want %v", names, want)
	}
	if b := byName["merged-local"]; !b.Merged || b.Gone || b.Hash == "" || b.When.IsZero() {
		t.Errorf("merged-local: %+v", b)
	}
	if b := byName["merged-gone"]; !b.Merged || !b.Gone {
		t.Errorf("merged-gone: %+v", b)
	}
	if b := byName["squashed"]; b.Merged || !b.Gone || b.Unmerged != 1 {
		t.Errorf("squashed: %+v", b)
	}

	if err := DeleteBranch(context.Background(), dir, "squashed"); err != nil {
		t.Fatal(err)
	}
	after, _, _ := StaleBranches(context.Background(), dir)
	if len(after) != 2 {
		t.Fatalf("after deleting squashed: %+v", after)
	}
}

func TestStaleBranchesWithoutRemote(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
		{"branch", "done"},
	} {
		if _, err := Git(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	got, base, err := StaleBranches(ctx, dir)
	if err != nil || base != "main" || len(got) != 1 || got[0].Name != "done" || !got[0].Merged {
		t.Fatalf("got %+v base=%q err=%v", got, base, err)
	}
}

// After a default-branch rename on the host, origin/HEAD can point at a
// ref that no longer exists. That must not break the check.
func TestStaleBranchesWithStaleOriginHead(t *testing.T) {
	dir := cleanupFixture(t)
	if _, err := Git(context.Background(), dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/master"); err != nil {
		t.Fatal(err)
	}
	got, base, err := StaleBranches(context.Background(), dir)
	if err != nil || base != "origin/main" || len(got) != 3 {
		t.Fatalf("got %d branches, base=%q, err=%v", len(got), base, err)
	}
}

package gitinfo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// removalFixture makes root/work, a clone of root/origin.git with one
// pushed commit, so it starts with nothing to lose.
func removalFixture(t *testing.T) (root, work string, git func(dir string, args ...string)) {
	t.Helper()
	root = t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work = filepath.Join(root, "work")
	ctx := context.Background()
	git = func(dir string, args ...string) {
		t.Helper()
		args = append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)
		if _, err := Git(ctx, dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	git(root, "init", "-q", "--bare", "-b", "main", origin)
	git(root, "clone", "-q", origin, work)
	git(work, "commit", "-q", "--allow-empty", "-m", "init")
	git(work, "push", "-q", "origin", "main")
	return root, work, git
}

func TestCheckRemovalCleanRepo(t *testing.T) {
	root, work, _ := removalFixture(t)
	r := CheckRemoval(context.Background(), root, work)
	if r.Blocked != "" || r.Risky() {
		t.Fatalf("a clean, pushed clone should be safe: %+v", r)
	}
}

func TestCheckRemovalFindsWork(t *testing.T) {
	root, work, git := removalFixture(t)
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(work, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore")
	write(".env")
	git(work, "commit", "-q", "--allow-empty", "-m", "unpushed")
	git(work, "checkout", "-q", "-b", "local-only")
	git(work, "commit", "-q", "--allow-empty", "-m", "also unpushed")
	write("stashed.txt")
	git(work, "add", "stashed.txt")
	git(work, "stash", "-q")
	write("new.txt")
	if err := os.WriteFile(filepath.Join(work, ".gitignore"), []byte(".env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(work, "vendor", "lib")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	git(nested, "init", "-q")

	r := CheckRemoval(context.Background(), root, work)
	if r.Blocked != "" || !r.Risky() {
		t.Fatalf("should be risky but allowed: %+v", r)
	}
	if r.Changes != 3 || r.Stashes != 1 || r.Unpushed != 2 || r.NoRemotes {
		t.Fatalf("changes=%d stashes=%d unpushed=%d noRemotes=%v, want 3 (.gitignore, new.txt, vendor/), 1, 2, false",
			r.Changes, r.Stashes, r.Unpushed, r.NoRemotes)
	}
	if strings.Join(r.Ignored, ",") != ".env" {
		t.Fatalf("ignored = %q", r.Ignored)
	}
	if strings.Join(r.Nested, ",") != "vendor/lib" {
		t.Fatalf("nested = %q", r.Nested)
	}
}

func TestCheckRemovalNoRemotes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "solo")
	ctx := context.Background()
	if _, err := Git(ctx, root, "init", "-q", dir); err != nil {
		t.Fatal(err)
	}
	r := CheckRemoval(ctx, root, dir)
	if r.Blocked != "" || !r.NoRemotes || !r.Risky() {
		t.Fatalf("an empty repo with no remotes should be allowed but risky: %+v", r)
	}
}

func TestCheckRemovalBlocks(t *testing.T) {
	root, work, git := removalFixture(t)
	ctx := context.Background()

	link := filepath.Join(root, "link")
	if err := os.Symlink(work, link); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, "wt")
	git(work, "worktree", "add", "-q", wt)

	cases := map[string]struct{ root, dir, want string }{
		"scan root":   {work, work, "scanning"},
		"outside":     {filepath.Join(root, "other"), work, "outside"},
		"sibling":     {filepath.Join(root, "wo"), work, "outside"},
		"relative":    {root, "work", "absolute"},
		"symlink":     {root, link, "symlink"},
		"worktree":    {root, wt, "linked worktree"},
		"has a wt":    {root, work, "linked worktree(s) depend on it"},
		"not a repo":  {root, filepath.Join(root, "origin.git"), "no .git"},
		"nonexistent": {root, filepath.Join(root, "gone"), "no such file"},
	}
	for name, c := range cases {
		r := CheckRemoval(ctx, c.root, c.dir)
		if !strings.Contains(r.Blocked, c.want) {
			t.Errorf("%s: Blocked = %q, want it to mention %q", name, r.Blocked, c.want)
		}
	}
}

package gitinfo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseStatus(t *testing.T) {
	out := `# branch.oid 1234567890abcdef
# branch.head main
# branch.upstream origin/main
# branch.ab +2 -3
# stash 1
1 M. N... 100644 100644 100644 aaa bbb staged.go
1 .M N... 100644 100644 100644 aaa bbb unstaged.go
1 MM N... 100644 100644 100644 aaa bbb both.go
2 R. N... 100644 100644 100644 aaa bbb R100 new.go	old.go
u UU N... 100644 100644 100644 100644 aaa bbb ccc conflict.go
? untracked.txt
? other.txt
`
	var s Status
	parseStatus([]byte(out), &s)

	want := Status{Branch: "main", Head: "1234567", Upstream: "origin/main", Ahead: 2, Behind: 3,
		Stashes: 1, Staged: 3, Unstaged: 2, Conflicts: 1, Untracked: 2}
	if s.Branch != want.Branch || s.Head != want.Head || s.Upstream != want.Upstream ||
		s.Ahead != want.Ahead || s.Behind != want.Behind || s.Stashes != want.Stashes ||
		s.Staged != want.Staged || s.Unstaged != want.Unstaged ||
		s.Conflicts != want.Conflicts || s.Untracked != want.Untracked {
		t.Fatalf("got %+v\nwant %+v", s, want)
	}
}

func TestParseStatusDetachedInitial(t *testing.T) {
	var s Status
	parseStatus([]byte("# branch.oid (initial)\n# branch.head (detached)\n"), &s)
	if s.Branch != "" || s.Head != "" || s.Dirty() {
		t.Fatalf("got %+v", s)
	}
}

func TestGetDetails(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if _, err := Git(context.Background(), dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init", "-q", "-b", "main")
	run("remote", "add", "origin", "git@github.com:o/r.git")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("[![ci](x)](y)\n# Title\n\n\n\nBody line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("-c", "user.name=Ada", "-c", "user.email=a@b", "commit", "-q", "-m", "first commit")
	run("branch", "feature")

	d := GetDetails(context.Background(), dir, 10, 10, 10)
	if len(d.Commits) != 1 || d.Commits[0].Subject != "first commit" || d.Commits[0].Author != "Ada" || d.Commits[0].When.IsZero() {
		t.Errorf("commits: %+v", d.Commits)
	}
	if len(d.Branches) != 2 {
		t.Errorf("branches: %+v", d.Branches)
	}
	if len(d.Remotes) != 1 || d.Remotes[0].Name != "origin" {
		t.Errorf("remotes: %+v", d.Remotes)
	}
	if want := []string{"# Title", "", "Body line"}; strings.Join(d.Readme, "|") != strings.Join(want, "|") {
		t.Errorf("readme: %q", d.Readme)
	}
}

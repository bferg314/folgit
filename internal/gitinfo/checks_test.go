package gitinfo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSSHURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r.git":      "git@github.com:o/r.git",
		"https://github.com/o/r":          "git@github.com:o/r.git",
		"https://github.com/o/r/":         "git@github.com:o/r.git",
		"https://me@github.com/o/r.git":   "git@github.com:o/r.git",
		"https://github.com/o/my.repo":    "git@github.com:o/my.repo.git",
		"git@github.com:o/r.git":          "",
		"https://gitlab.com/o/r.git":      "",
		"https://github.com/o/r/tree/x":   "",
		"https://github.com.evil/o/r.git": "",
	} {
		got, ok := SSHURL(in)
		if got != want || ok != (want != "") {
			t.Errorf("SSHURL(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

// eolFixture is a repo that never converts line endings itself, so tests
// don't depend on the machine's core.autocrlf.
func eolFixture(t *testing.T) (dir string, git func(args ...string) string, write func(name, content string)) {
	t.Helper()
	dir = t.TempDir()
	git = func(args ...string) string {
		t.Helper()
		out, err := Git(context.Background(), dir, args...)
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return string(out)
	}
	write = func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "core.autocrlf", "false")
	git("config", "user.name", "Ada")
	git("config", "user.email", "a@b")
	return dir, git, write
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHTTPSRemotesAndUseSSH(t *testing.T) {
	dir, git, _ := eolFixture(t)
	git("remote", "add", "origin", "https://github.com/o/r.git")
	git("remote", "add", "upstream", "git@github.com:u/r.git")
	git("remote", "add", "other", "https://gitlab.com/o/r.git")

	ctx := context.Background()
	rs, err := HTTPSRemotes(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Name != "origin" || rs[0].SSHURL != "git@github.com:o/r.git" {
		t.Fatalf("got %+v", rs)
	}
	if err := UseSSH(ctx, dir, rs); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(git("remote", "get-url", "origin")); got != "git@github.com:o/r.git" {
		t.Fatalf("origin is %q", got)
	}
	if rs, _ := HTTPSRemotes(ctx, dir); len(rs) != 0 {
		t.Fatalf("still HTTPS: %+v", rs)
	}
}

func TestHTTPSRemotesNone(t *testing.T) {
	dir, _, _ := eolFixture(t)
	rs, err := HTTPSRemotes(context.Background(), dir)
	if err != nil || len(rs) != 0 {
		t.Fatalf("got %+v, %v", rs, err)
	}
}

func TestFixCommittedCRLF(t *testing.T) {
	dir, git, write := eolFixture(t)
	write("crlf.txt", "one\r\ntwo\r\n")
	write("lf.txt", "one\ntwo\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")

	ctx := context.Background()
	l, err := CheckLineEndings(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if l.Attributes || !slices.Equal(l.Committed, []string{"crlf.txt"}) || len(l.CheckedOut) != 0 {
		t.Fatalf("got %+v", l)
	}

	fix, err := FixLineEndings(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if fix != (LineEndingFix{AddedAttributes: true, Renormalized: 1, Rewritten: 1}) {
		t.Fatalf("got %+v", fix)
	}
	if got := readFile(t, filepath.Join(dir, ".gitattributes")); got != lfAttributes {
		t.Fatalf(".gitattributes is %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "crlf.txt")); got != "one\ntwo\n" {
		t.Fatalf("crlf.txt is %q", got)
	}
	// The fix is left for the user to commit.
	if got := git("status", "--porcelain"); got != "M  crlf.txt\n?? .gitattributes\n" {
		t.Fatalf("status:\n%s", got)
	}
	if l, _ := CheckLineEndings(ctx, dir); !l.OK() {
		t.Fatalf("still flagged: %+v", l)
	}
}

func TestFixCheckedOutCRLF(t *testing.T) {
	dir, git, write := eolFixture(t)
	write("a.txt", "one\ntwo\n")
	write("run.bat", "echo\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	// Git for Windows checks the files out with CRLF, then .gitattributes
	// arrives asking for LF (but CRLF for .bat).
	for _, name := range []string{"a.txt", "run.bat"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	git("-c", "core.autocrlf=true", "checkout", "--", ".")
	write(".gitattributes", "* text=auto eol=lf\n*.bat text eol=crlf\n")
	git("add", ".gitattributes")
	git("commit", "-q", "-m", "attributes")

	ctx := context.Background()
	l, err := CheckLineEndings(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Attributes || len(l.Committed) != 0 || !slices.Equal(l.CheckedOut, []string{"a.txt"}) {
		t.Fatalf("got %+v", l)
	}
	fix, err := FixLineEndings(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if fix != (LineEndingFix{Rewritten: 1}) {
		t.Fatalf("got %+v", fix)
	}
	if got := readFile(t, filepath.Join(dir, "a.txt")); got != "one\ntwo\n" {
		t.Fatalf("a.txt is %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "run.bat")); got != "echo\r\n" {
		t.Fatalf("run.bat is %q", got)
	}
	if got := git("status", "--porcelain"); got != "" {
		t.Fatalf("status:\n%s", got)
	}
}

func TestLineEndingsLeaveNonTextAlone(t *testing.T) {
	dir, git, write := eolFixture(t)
	write(".gitattributes", "*.dat -text\n")
	write("x.dat", "raw\r\n")
	write("notes.txt", "a\r\n") // no eol rule for it, so its CRLF checkout is a choice
	git("add", ".")
	git("commit", "-q", "-m", "first")
	l, err := CheckLineEndings(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	// notes.txt is committed with CRLF (attributes don't mark it as text,
	// but nothing marks it as binary either), x.dat is binary on purpose.
	if !slices.Equal(l.Committed, []string{"notes.txt"}) || len(l.CheckedOut) != 0 {
		t.Fatalf("got %+v", l)
	}
}

func TestFixLineEndingsRefusesUncommitted(t *testing.T) {
	dir, git, write := eolFixture(t)
	write("crlf.txt", "one\r\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	write("crlf.txt", "changed\r\n")
	if _, err := FixLineEndings(context.Background(), dir); !errors.Is(err, ErrUncommitted) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitattributes")); err == nil {
		t.Fatal(".gitattributes was written")
	}
}

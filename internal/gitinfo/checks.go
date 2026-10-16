package gitinfo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// HTTPSRemote is a remote whose URL is HTTPS but could be SSH.
type HTTPSRemote struct {
	Name   string
	URL    string
	SSHURL string
}

var githubHTTPS = regexp.MustCompile(`^https://(?:[^@/]+@)?github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$`)

// SSHURL returns the SSH form of an HTTPS GitHub URL. Other hosts are left
// alone: their SSH user, host and port can't be guessed from the HTTPS URL.
func SSHURL(url string) (string, bool) {
	m := githubHTTPS.FindStringSubmatch(url)
	if m == nil {
		return "", false
	}
	return "git@github.com:" + m[1] + "/" + m[2] + ".git", true
}

// HTTPSRemotes lists the repo's remotes that use HTTPS GitHub URLs.
func HTTPSRemotes(ctx context.Context, dir string) ([]HTTPSRemote, error) {
	out, err := Git(ctx, dir, "config", "--get-regexp", `^remote\..*\.url$`)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil // no remotes
		}
		return nil, err
	}
	var rs []HTTPSRemote
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		key, url, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url")
		if ssh, ok := SSHURL(url); ok {
			rs = append(rs, HTTPSRemote{Name: name, URL: url, SSHURL: ssh})
		}
	}
	return rs, nil
}

// UseSSH switches the given remotes to their SSH URLs.
func UseSSH(ctx context.Context, dir string, rs []HTTPSRemote) error {
	for _, r := range rs {
		if _, err := Git(ctx, dir, "remote", "set-url", r.Name, r.SSHURL); err != nil {
			return err
		}
	}
	return nil
}

// LineEndings describes where a repo's text files have CRLF line endings
// that its attributes (or the lack of any) don't call for.
type LineEndings struct {
	// Attributes is whether the repo has a top-level .gitattributes.
	Attributes bool
	// Committed are text files stored in the repo with CRLF.
	Committed []string
	// CheckedOut are files stored with LF but checked out with CRLF, where
	// .gitattributes asks for LF or there is no .gitattributes at all.
	CheckedOut []string
}

// OK reports whether nothing needs fixing.
func (l LineEndings) OK() bool { return len(l.Committed) == 0 && len(l.CheckedOut) == 0 }

// lfAttributes is what FixLineEndings writes when a repo has no
// .gitattributes: git decides which files are text and stores and checks
// them out with LF everywhere.
const lfAttributes = "* text=auto eol=lf\n"

// CheckLineEndings reads `git ls-files --eol` for the repo at dir.
func CheckLineEndings(ctx context.Context, dir string) (LineEndings, error) {
	var l LineEndings
	if _, err := os.Stat(filepath.Join(dir, ".gitattributes")); err == nil {
		l.Attributes = true
	}
	files, err := eolFiles(ctx, dir)
	if err != nil {
		return l, err
	}
	for _, f := range files {
		switch {
		case f.committedCRLF():
			l.Committed = append(l.Committed, f.path)
		case f.index == "lf" && f.worktree == "crlf" && (!l.Attributes || f.wantsLF()):
			l.CheckedOut = append(l.CheckedOut, f.path)
		}
	}
	return l, nil
}

type eolFile struct {
	index, worktree, attr, path string
}

func (f eolFile) committedCRLF() bool {
	return (f.index == "crlf" || f.index == "mixed") && !strings.Contains(f.attr, "-text")
}

func (f eolFile) wantsLF() bool { return strings.Contains(f.attr, "eol=lf") }

// eolFiles parses `git ls-files --eol -z`: "i/lf    w/crlf  attr/text=auto eol=lf \tpath".
func eolFiles(ctx context.Context, dir string) ([]eolFile, error) {
	out, err := Git(ctx, dir, "ls-files", "--eol", "-z")
	if err != nil {
		return nil, err
	}
	var files []eolFile
	for _, rec := range strings.Split(string(out), "\x00") {
		info, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		f := eolFile{path: path}
		for _, field := range strings.Fields(info) {
			switch {
			case strings.HasPrefix(field, "i/"):
				f.index = field[2:]
			case strings.HasPrefix(field, "w/"):
				f.worktree = field[2:]
			}
		}
		// attr/ can hold spaces ("text=auto eol=lf"), so take the rest of the line.
		if _, attr, ok := strings.Cut(info, "attr/"); ok {
			f.attr = strings.TrimSpace(attr)
		}
		files = append(files, f)
	}
	return files, nil
}

// LineEndingFix is what FixLineEndings changed.
type LineEndingFix struct {
	AddedAttributes bool
	Renormalized    int // files restaged with LF
	Rewritten       int // working tree files rewritten with LF
}

// ErrUncommitted is returned when a fix would mix with the user's own changes.
var ErrUncommitted = errors.New("commit or stash your changes first")

// FixLineEndings makes the repo store and check out text files with LF.
// It adds a .gitattributes if there is none, restages files committed with
// CRLF (left staged for the user to review and commit), and rewrites
// working tree files that only differ from the repo in line endings. It
// refuses to run with uncommitted changes to tracked files, so nothing of
// the user's is staged or overwritten.
func FixLineEndings(ctx context.Context, dir string) (LineEndingFix, error) {
	var fix LineEndingFix
	out, err := Git(ctx, dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return fix, err
	}
	if len(bytes.TrimSpace(out)) > 0 {
		return fix, ErrUncommitted
	}

	attrs := filepath.Join(dir, ".gitattributes")
	if _, err := os.Stat(attrs); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(attrs, []byte(lfAttributes), 0o644); err != nil {
			return fix, err
		}
		fix.AddedAttributes = true
	}

	files, err := eolFiles(ctx, dir)
	if err != nil {
		return fix, err
	}
	var renorm []string
	for _, f := range files {
		if f.committedCRLF() {
			renorm = append(renorm, f.path)
		}
	}
	if len(renorm) > 0 {
		// Literal pathspecs, so names with * or ? match only themselves.
		if err := gitStdin(ctx, dir, renorm, "--literal-pathspecs", "add", "--renormalize",
			"--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
			return fix, err
		}
		fix.Renormalized = len(renorm)
	}

	// List again: the new .gitattributes and the restaged files change it.
	if files, err = eolFiles(ctx, dir); err != nil {
		return fix, err
	}
	var rewrite []string
	for _, f := range files {
		if f.index == "lf" && f.worktree == "crlf" && f.wantsLF() {
			rewrite = append(rewrite, f.path)
		}
	}
	if len(rewrite) > 0 {
		// checkout-index skips files whose stat info matches the index,
		// even with --force, and these do: git wrote them itself. An mtime
		// that doesn't match makes it look at them; it rewrites them all.
		stale := time.Unix(1, 0)
		for _, p := range rewrite {
			if err := os.Chtimes(filepath.Join(dir, filepath.FromSlash(p)), stale, stale); err != nil {
				return fix, err
			}
		}
		if err := gitStdin(ctx, dir, rewrite, "checkout-index", "--force", "-u", "-z", "--stdin"); err != nil {
			return fix, err
		}
		fix.Rewritten = len(rewrite)
	}
	return fix, nil
}

// gitStdin runs git in dir with paths on stdin, NUL-separated.
func gitStdin(ctx context.Context, dir string, paths []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return &Error{Msg: msg, Err: err}
		}
		return err
	}
	return nil
}

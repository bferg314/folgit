package launcher

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bferg314/folgit/internal/config"
)

func TestGitErrorLine(t *testing.T) {
	cases := map[string]string{
		"fatal: 'x' does not appear to be a git repository\nfatal: Could not read from remote repository.\n\nPlease make sure you have the correct access rights\nand the repository exists.\n": "'x' does not appear to be a git repository",
		"hint: something\nerror: cannot pull with rebase\n": "cannot pull with rebase",
		"Not possible to fast-forward, aborting.\n":         "Not possible to fast-forward, aborting.",
	}
	for in, want := range cases {
		if got := gitErrorLine(in); got != want {
			t.Errorf("gitErrorLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestZellijTab(t *testing.T) {
	tool := config.Tool{Name: "Git", Cmd: "git", Args: []string{"-C", "{path}", "status"}}
	dir := t.TempDir()
	cmd := ZellijTab(tool, dir)
	args := cmd.Args[1:]
	sep := slices.Index(args, "--")
	if sep < 0 {
		t.Fatalf("no -- in %q", args)
	}
	for _, flag := range []string{"new-tab", "--close-on-exit", "--block-until-exit", "--cwd"} {
		if !slices.Contains(args[:sep], flag) {
			t.Errorf("missing %s in %q", flag, args[:sep])
		}
	}
	if want := filepath.Base(dir) + " · Git"; args[slices.Index(args, "--name")+1] != want {
		t.Errorf("tab name = %q, want %q", args[slices.Index(args, "--name")+1], want)
	}
	if got, want := args[sep+2:], []string{"-C", dir, "status"}; !slices.Equal(got, want) {
		t.Errorf("tool args = %q, want %q", got, want)
	}
}

func TestZellijError(t *testing.T) {
	if got := ZellijError([]byte("\nerror: unexpected argument '--block-until-exit'\n\nUsage: ...\n"), errors.New("exit status 2")); got.Error() != "error: unexpected argument '--block-until-exit'" {
		t.Errorf("got %q", got)
	}
	if got := ZellijError(nil, errors.New("exit status 1")); got.Error() != "exit status 1" {
		t.Errorf("got %q", got)
	}
}

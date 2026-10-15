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

func TestFindHerdrWorkspace(t *testing.T) {
	list := []byte(`{"id":"cli:workspace:list","result":{"type":"workspace_list","workspaces":[` +
		`{"label":"ninja","workspace_id":"w4"},{"label":"folgit","workspace_id":"w7"}]}}`)
	if got := findHerdrWorkspace(list, "folgit"); got != "w7" {
		t.Errorf("got %q, want w7", got)
	}
	if got := findHerdrWorkspace(list, "other"); got != "" {
		t.Errorf("got %q, want none", got)
	}
	if got := findHerdrWorkspace([]byte("not json"), "folgit"); got != "" {
		t.Errorf("got %q from bad output, want none", got)
	}
}

func TestHerdrCommandLine(t *testing.T) {
	bare := config.Tool{Cmd: "claude"}
	withPath := config.Tool{Cmd: "code", Args: []string{"{path}"}}
	cases := []struct {
		tool       config.Tool
		dir, shell string
		want       string
	}{
		{bare, `C:\code\x`, "pwsh", "claude"},
		{bare, "/code/x", "bash", "claude"},
		{withPath, `C:\code\x`, "pwsh", `code C:\code\x`},
		{withPath, `C:\My Code\it's`, "pwsh", `code 'C:\My Code\it''s'`},
		{withPath, `C:\My Code`, "cmd", `code "C:\My Code"`},
		{withPath, "/code/it's here", "zsh", `code '/code/it'\''s here'`},
		{withPath, `C:\code\x`, "bash", `code 'C:\code\x'`},
		{config.Tool{Cmd: "my tool"}, "/x", "pwsh", "& 'my tool'"},
	}
	for _, c := range cases {
		if got := herdrCommandLine(c.tool, c.dir, c.shell); got != c.want {
			t.Errorf("herdrCommandLine(%q, %q, %q) = %q, want %q", c.tool.Cmd, c.dir, c.shell, got, c.want)
		}
	}
}

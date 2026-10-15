// Package launcher builds commands for external tools and git operations.
package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/provider"
)

// Command builds the exec.Cmd for tool t in dir, expanding {path}.
func Command(t config.Tool, dir string) *exec.Cmd {
	args := make([]string, len(t.Args))
	for i, a := range t.Args {
		args[i] = strings.ReplaceAll(a, "{path}", dir)
	}
	cmd := exec.Command(t.Cmd, args...)
	cmd.Dir = dir
	return cmd
}

// InZellij reports whether folgit is running inside a zellij session.
func InZellij() bool {
	return os.Getenv("ZELLIJ") != ""
}

// ZellijTab builds a command that runs tool t in a new zellij tab in dir.
// It blocks until the tool exits, and the tab closes with it.
func ZellijTab(t config.Tool, dir string) *exec.Cmd {
	tool := Command(t, dir)
	args := []string{"action", "new-tab",
		"--name", filepath.Base(dir) + " · " + t.Name,
		"--cwd", dir,
		"--close-on-exit", "--block-until-exit",
		// The resolved path, so zellij runs what folgit found on PATH.
		"--", tool.Path}
	cmd := exec.Command("zellij", append(args, tool.Args[1:]...)...)
	cmd.Dir = dir
	return cmd
}

// ZellijError turns a failed zellij action's output into an error.
func ZellijError(out []byte, err error) error {
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if line = strings.TrimSpace(line); line != "" {
		return errors.New(line)
	}
	return err
}

// InHerdr reports whether folgit is running inside a herdr pane.
func InHerdr() bool {
	return os.Getenv("HERDR_ENV") == "1"
}

// HerdrWorkspace starts tool t in a herdr workspace for dir, labelled after
// the repo. If one is already open it is focused and the tool gets a new tab
// there; otherwise the workspace is created. The tool is typed into the
// pane's shell and keeps running there, so unlike ZellijTab nothing waits
// for it to exit. It reports whether the workspace already existed.
func HerdrWorkspace(t config.Tool, dir string) (existing bool, err error) {
	label := filepath.Base(dir)
	list, err := herdr("workspace", "list")
	if err != nil {
		return false, err
	}
	var pane struct {
		Result struct {
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	var out []byte
	if ws := findHerdrWorkspace(list, label); ws != "" {
		existing = true
		if _, err := herdr("workspace", "focus", ws); err != nil {
			return true, err
		}
		out, err = herdr("tab", "create", "--workspace", ws, "--cwd", dir, "--label", t.Name, "--focus")
	} else {
		out, err = herdr("workspace", "create", "--cwd", dir, "--label", label, "--focus")
	}
	if err != nil {
		return existing, err
	}
	if err := json.Unmarshal(out, &pane); err != nil || pane.Result.RootPane.PaneID == "" {
		return existing, fmt.Errorf("unexpected herdr output: %s", strings.TrimSpace(string(out)))
	}
	id := pane.Result.RootPane.PaneID
	line := herdrCommandLine(t, dir, herdrShell(id))
	_, err = herdr("pane", "run", id, line)
	return existing, err
}

// herdr runs a herdr CLI command and returns its JSON output. Failures are
// reported by herdr as JSON on stderr, which becomes the error.
func herdr(args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("herdr", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var resp struct {
			Error struct{ Message string } `json:"error"`
		}
		if json.Unmarshal(stderr.Bytes(), &resp) == nil && resp.Error.Message != "" {
			return nil, errors.New(resp.Error.Message)
		}
		return nil, ZellijError(stderr.Bytes(), err)
	}
	return out, nil
}

// findHerdrWorkspace returns the ID of the workspace labelled label in the
// output of `herdr workspace list`, or "".
func findHerdrWorkspace(list []byte, label string) string {
	var resp struct {
		Result struct {
			Workspaces []struct {
				ID    string `json:"workspace_id"`
				Label string `json:"label"`
			} `json:"workspaces"`
		} `json:"result"`
	}
	if json.Unmarshal(list, &resp) != nil {
		return ""
	}
	for _, w := range resp.Result.Workspaces {
		if w.Label == label {
			return w.ID
		}
	}
	return ""
}

// herdrShell names the shell running in pane (e.g. "pwsh", "bash"), or ""
// if herdr can't say.
func herdrShell(pane string) string {
	out, err := herdr("pane", "process-info", "--pane", pane)
	if err != nil {
		return ""
	}
	var resp struct {
		Result struct {
			ProcessInfo struct {
				Processes []struct{ Name string } `json:"foreground_processes"`
			} `json:"process_info"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &resp) != nil || len(resp.Result.ProcessInfo.Processes) == 0 {
		return ""
	}
	name := strings.ToLower(resp.Result.ProcessInfo.Processes[0].Name)
	return strings.TrimSuffix(name, ".exe")
}

// herdrCommandLine is tool t in dir as a command line for shell. The bare
// command name is used, as the shell finds it on PATH just as folgit did.
// An unknown shell gets the platform's usual one: PowerShell on Windows,
// a POSIX shell elsewhere.
func herdrCommandLine(t config.Tool, dir, shell string) string {
	if shell == "" {
		shell = "sh"
		if runtime.GOOS == "windows" {
			shell = "pwsh"
		}
	}
	words := []string{t.Cmd}
	for _, a := range t.Args {
		words = append(words, strings.ReplaceAll(a, "{path}", dir))
	}
	for i, w := range words {
		words[i] = shellQuote(shell, w)
	}
	line := strings.Join(words, " ")
	// PowerShell treats a quoted first word as a string, not a command.
	if (shell == "pwsh" || shell == "powershell") && strings.HasPrefix(line, "'") {
		line = "& " + line
	}
	return line
}

// shellQuote quotes w for shell when it has anything beyond plain path and
// word characters.
func shellQuote(shell, w string) string {
	switch shell {
	case "pwsh", "powershell":
		if w != "" && !strings.ContainsFunc(w, notIn(`-_./\:=+,@%`)) {
			return w
		}
		return "'" + strings.ReplaceAll(w, "'", "''") + "'"
	case "cmd":
		if w != "" && !strings.ContainsFunc(w, notIn(`-_./\:=+,@`)) {
			return w
		}
		return `"` + w + `"`
	default:
		if w != "" && !strings.ContainsFunc(w, notIn(`-_./:=+,@%`)) {
			return w
		}
		return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
	}
}

// notIn matches runes that are neither letters, digits nor in extra.
func notIn(extra string) func(rune) bool {
	return func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(extra, r))
	}
}

// Detach starts cmd without waiting and without tying it to our terminal.
func Detach(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detachAttrs(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// OpenURL opens url in the default browser.
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Avoids cmd.exe's "start", which mangles URLs containing "&".
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return Detach(cmd)
}

// CloneDest expands layout ("{host}/{owner}/{repo}") under root.
func CloneDest(root, layout string, r provider.Repo) string {
	if layout == "" {
		layout = "{repo}"
	}
	rel := strings.NewReplacer("{host}", r.Host, "{owner}", r.Owner, "{repo}", r.Name).Replace(layout)
	return filepath.Join(root, filepath.FromSlash(rel))
}

// Clone runs `git clone` into dest. It refuses to overwrite existing paths.
func Clone(ctx context.Context, url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return runGit(ctx, "", "clone", "--quiet", url, dest)
}

// Pull fast-forwards the current branch from its upstream.
func Pull(ctx context.Context, dir string) error {
	return runGit(ctx, dir, "pull", "--ff-only", "--quiet")
}

// Switch checks out branch. With track, branch is a remote branch
// ("origin/feature") and a local branch tracking it is created.
func Switch(ctx context.Context, dir, branch string, track bool) error {
	if track {
		return runGit(ctx, dir, "switch", "--quiet", "--track", branch)
	}
	return runGit(ctx, dir, "switch", "--quiet", branch)
}

// Fetch updates every remote of the repo in dir, pruning deleted branches.
func Fetch(ctx context.Context, dir string) error {
	return runGit(ctx, dir, "fetch", "--all", "--prune", "--quiet")
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = nonInteractiveEnv(ctx, dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := gitErrorLine(string(out)); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// nonInteractiveEnv stops git and ssh from prompting: folgit owns the
// terminal, so a prompt would hang or draw over the UI. HTTPS prompts are
// disabled outright. SSH is put in batch mode (an unlocked agent still
// works) unless the user has their own SSH command configured, which we
// must not override.
func nonInteractiveEnv(ctx context.Context, dir string) []string {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if os.Getenv("GIT_SSH_COMMAND") != "" || os.Getenv("GIT_SSH") != "" {
		return env
	}
	check := exec.CommandContext(ctx, "git", "config", "--get", "core.sshCommand")
	check.Dir = dir
	if out, err := check.Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		return env
	}
	return append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
}

// gitErrorLine picks the most useful line from git's output: the first
// "fatal:" or "error:" line (git follows these with generic advice), or
// else the last line.
func gitErrorLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		for _, p := range []string{"fatal: ", "error: "} {
			if msg, ok := strings.CutPrefix(l, p); ok {
				return msg
			}
		}
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

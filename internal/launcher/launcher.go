// Package launcher builds commands for external tools and git operations.
package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// Detach starts cmd without waiting and without tying it to our terminal.
func Detach(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detachAttrs(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
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

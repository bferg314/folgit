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

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// Never let git prompt for credentials: there is no terminal to answer.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := lastLine(string(out)); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

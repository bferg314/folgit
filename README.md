# folgit

A terminal dashboard for every git repository under a directory, plus the GitHub repositories you haven't cloned yet.

```
folgit            # scan the current directory
folgit ~/code     # scan somewhere else
```

## Install

You need Go 1.26 or newer and `git` on your PATH.

```sh
go install github.com/bferg314/folgit/cmd/folgit@latest
```

This puts the `folgit` binary in `$(go env GOPATH)/bin`, which is `~/go/bin` or `%USERPROFILE%\go\bin` unless you've changed it. Make sure that folder is on your PATH. Run the same command again to update.

For the Remote tab, sign in with the [GitHub CLI](https://cli.github.com/) (`gh auth login`) or set `GITHUB_TOKEN`. See [GitHub auth](#github-auth).

## Tabs

- **Local**: every repo found, with its branch, status and the time of its last commit. Press `enter` to choose a tool to open it in, or press a tool's key directly. `g` quits and moves your shell into the repo (see [Shell integration](#shell-integration)). `r` rescans, `s` changes the sort order, `/` filters.
  - **Pulling:** `p` pulls the selected repo (fast-forward only). `F` fetches every repo, 8 at a time, so the ↓ counts are current. `P` fetches every repo too, then pulls the ones that are behind and have no local changes or unpushed commits. Everything else is left alone and counted as skipped.
  - **Detail pane:** on terminals at least 100 columns wide, a pane beside the list shows the selected repo's working tree state, recent commits, branches (including ones whose upstream is gone or that exist only locally), remotes and the start of its README. `d` hides it. On narrower terminals `d` shows it full screen and `esc` closes it.
  - **Scrolling the details:** `J`/`K` scroll the pane a line at a time and `ctrl+d`/`ctrl+u` half a page. The repo name, branch and change summary stay at the top, and a percentage beside the name shows your position. In the full-screen view, `j`/`k`, the arrow keys and `pgup`/`pgdn` scroll as well. The mouse wheel scrolls whichever side it is over. Because folgit now receives mouse events, most terminals need `shift` held to select text.
  - **Issues:** `i` opens the selected repo's open GitHub issues, most recently updated first, with labels and when each was last updated. `j`/`k` move, `enter` opens the issue in your browser, `r` refreshes and `esc` closes. Pull requests are left out. folgit tries each GitHub remote in turn, so a fork with issues turned off falls back to its upstream remote. `i` works on the Remote tab too.
- **Remote**: your GitHub repos that aren't on this machine. `space` selects, `a` selects all, `enter` clones into the scanned directory.
- **Settings**: turn tools and GitHub listing options on and off.

Press `?` for the full key list and what the status symbols mean.

## Shell integration

A program can't change its parent shell's directory, so folgit ships a small wrapper function. Add the line for your shell to its startup file:

```sh
eval "$(folgit init bash)"                                # ~/.bashrc
eval "$(folgit init zsh)"                                 # ~/.zshrc
folgit init fish | source                                 # ~/.config/fish/config.fish
Invoke-Expression (& folgit init powershell | Out-String)  # $PROFILE
```

After that, select a repo and press `g`: folgit exits and your shell is in that repo.

## Startup cache

folgit saves the last scan and remote list for each directory in your OS cache folder. On the next start it shows them right away, then refreshes in the background, dropping repos that have been deleted since. Run `folgit --no-cache` to ignore the cache.

## GitHub auth

folgit uses `GH_TOKEN` or `GITHUB_TOKEN` if either is set. Otherwise it asks `gh auth token`. Clones use `gh`'s `git_protocol` setting (ssh or https) unless `github.protocol` is set in the config.

## Config

On first run folgit writes `config.toml` to your OS config directory (`%AppData%\folgit` on Windows, `~/Library/Application Support/folgit` on macOS, `~/.config/folgit` on Linux) and switches on every tool it finds on your PATH.

```toml
max_depth = 4
clone_layout = "{repo}"          # or "{owner}/{repo}", "{host}/{owner}/{repo}"

[[tools]]
name = "Claude Code"
cmd = "claude"
args = []
key = "c"
mode = "terminal"                # suspends folgit until the tool exits
enabled = true

[[tools]]
name = "VS Code"
cmd = "code"
args = ["{path}"]
key = "o"
mode = "detach"                  # starts in the background
enabled = true
```

## Build

```
go build ./cmd/folgit
go test ./...
```

## Known limitations

- Clones, fetches and pulls run in the background with no terminal to type into, so they can't ask for passwords or SSH passphrases. They fail with an error instead: load your SSH key into an agent (or use a credential helper for HTTPS). If you set `core.sshCommand`, `GIT_SSH` or `GIT_SSH_COMMAND`, folgit uses it unchanged.
- Changing a setting in the Settings tab rewrites `config.toml`, which removes any comments you added by hand.
- GitHub is the only hosting service supported so far.

## Roadmap

Possible next steps, roughly in priority order:

- **Unpushed-work report.** One view of every repo with uncommitted changes, unpushed commits, stashes or branches that exist only on this machine, so you can tell whether it's safe to wipe it.
- **Releases.** A GoReleaser config and a GitHub Actions workflow so that tagging a version publishes binaries for Windows, macOS and Linux, plus Homebrew and Scoop packages. Installing would no longer need Go.
- **PR and CI badges.** Open pull requests and the latest CI result for each repo, using the same GitHub token.
- **Scriptable commands.** Non-interactive output such as `folgit ls --dirty --json` for scripts and shell prompts.
- **Branch cleanup.** Find local branches that are already merged or whose remote branch is gone, and delete them in bulk.
- **Pinned and hidden repos.** Keep favourites at the top and hide old experiments without deleting them.
- **Worktrees.** List each repo's worktrees under it.
- **Moving machines.** `folgit export` writes a list of your repos; `folgit import` clones the same set on another machine.
- **Disk usage and archiving.** Show each repo's size, and delete a local copy once everything in it is pushed.
- **More hosts.** GitLab, Gitea or Bitbucket, if they're ever needed.

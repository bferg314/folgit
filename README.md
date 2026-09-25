# folgit

A terminal dashboard for every git repository under a directory, plus the GitHub repositories you haven't cloned yet.

```
folgit            # scan the current directory
folgit ~/code     # scan somewhere else
```

## Tabs

- **Local**: every repo found, with its branch, status and the time of its last commit. Press `enter` to choose a tool to open it in, or press a tool's key directly. `g` quits and moves your shell into the repo (see [Shell integration](#shell-integration)). `p` pulls (fast-forward only), `r` rescans, `s` changes the sort order, `/` filters.
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

// Package shell produces wrapper functions that let folgit change the
// calling shell's directory. A child process can't change its parent's
// working directory, so the wrapper passes --cwd-file, folgit writes the
// chosen repo path there on exit, and the wrapper cd's into it.
package shell

import (
	"fmt"
	"sort"
	"strings"
)

// Names lists the supported shells.
func Names() []string {
	names := make([]string, 0, len(scripts))
	for n := range scripts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Script returns the wrapper for shell.
func Script(shell string) (string, error) {
	s, ok := scripts[strings.ToLower(shell)]
	if !ok {
		return "", fmt.Errorf("unsupported shell %q (supported: %s)", shell, strings.Join(Names(), ", "))
	}
	return s, nil
}

// Install explains how to hook the wrapper into shell's startup file.
func Install(shell string) string {
	switch strings.ToLower(shell) {
	case "bash":
		return `eval "$(folgit init bash)"    # add to ~/.bashrc`
	case "zsh":
		return `eval "$(folgit init zsh)"     # add to ~/.zshrc`
	case "fish":
		return `folgit init fish | source     # add to ~/.config/fish/config.fish`
	case "powershell", "pwsh":
		return `Invoke-Expression (& folgit init powershell | Out-String)    # add to $PROFILE`
	}
	return ""
}

const posix = `# folgit shell integration: press "g" in folgit to cd into the selected repo.
folgit() {
    local tmp dir
    tmp="$(mktemp -t folgit-cwd.XXXXXX)" || return
    command folgit --cwd-file="$tmp" "$@"
    local rc=$?
    dir="$(cat -- "$tmp" 2>/dev/null)"
    rm -f -- "$tmp"
    if [ -n "$dir" ] && [ "$dir" != "$PWD" ]; then
        builtin cd -- "$dir" || return
    fi
    return $rc
}
`

const fish = `# folgit shell integration: press "g" in folgit to cd into the selected repo.
function folgit
    set -l tmp (mktemp -t folgit-cwd.XXXXXX); or return
    command folgit --cwd-file=$tmp $argv
    set -l st $status
    set -l dir (cat -- $tmp 2>/dev/null)
    rm -f -- $tmp
    if test -n "$dir"; and test "$dir" != "$PWD"
        builtin cd -- $dir
    end
    return $st
end
`

const powershell = `# folgit shell integration: press "g" in folgit to cd into the selected repo.
function folgit {
    $exe = Get-Command folgit -CommandType Application -ErrorAction Stop | Select-Object -First 1
    $tmp = [System.IO.Path]::GetTempFileName()
    try {
        & $exe.Source "--cwd-file=$tmp" @args
        $dir = Get-Content -Raw -LiteralPath $tmp -ErrorAction SilentlyContinue
        if ($dir) {
            $dir = $dir.Trim()
            if ($dir -and $dir -ne $PWD.ProviderPath) { Set-Location -LiteralPath $dir }
        }
    } finally {
        Remove-Item -LiteralPath $tmp -ErrorAction SilentlyContinue
    }
}
`

var scripts = map[string]string{
	"bash":       posix,
	"zsh":        posix,
	"fish":       fish,
	"powershell": powershell,
	"pwsh":       powershell,
}

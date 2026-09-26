package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/launcher"
	"github.com/bferg314/folgit/internal/match"
)

// openURL opens a link in the default browser; tests replace it.
var openURL = launcher.OpenURL

// openWeb opens a repo's web page in the default browser.
func (a *App) openWeb(name, url string) tea.Cmd {
	if url == "" {
		return a.notify(2, "%s has no remote with a web page", name)
	}
	if err := openURL(url); err != nil {
		return a.notify(2, "Opening %s: %v", url, err)
	}
	return a.notify(1, "Opened %s in your browser", strings.TrimPrefix(url, "https://"))
}

// localWeb opens the selected local repo's page, from its first remote
// that looks like a hosted repo.
func (a *App) localWeb() tea.Cmd {
	r := a.local.selected()
	if r == nil {
		return nil
	}
	var url string
	if r.status != nil {
		for _, remote := range r.status.Remotes {
			if url = match.WebURL(remote); url != "" {
				break
			}
		}
	}
	return a.openWeb(r.rel, url)
}

// remoteWeb opens the selected remote repo's page.
func (a *App) remoteWeb() tea.Cmd {
	r := a.remote.current()
	if r == nil {
		return nil
	}
	return a.openWeb(r.FullName(), r.WebURL)
}

package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/provider"
)

// captureBrowser records opened URLs instead of launching a browser.
func captureBrowser(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	old := openURL
	openURL = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openURL = old })
	return &opened
}

func TestWOpensWebPage(t *testing.T) {
	opened := captureBrowser(t)
	a := newDetailApp(t, 120, 24)
	w := tea.KeyPressMsg{Code: 'w', Text: "w"}

	// Local: the first remote with a web page, SSH converted to https.
	a.local.selected().status.Remotes = []string{"/some/local/mirror", "git@github.com:Me/Alpha.git"}
	a.Update(w)
	if len(*opened) != 1 || (*opened)[0] != "https://github.com/Me/Alpha" {
		t.Fatalf("opened %v", *opened)
	}
	if !strings.Contains(a.toast.text, "github.com/Me/Alpha") {
		t.Errorf("toast = %q", a.toast.text)
	}

	// No usable remote: a message, no browser.
	a.local.selected().status = &gitinfo.Status{Branch: "main"}
	a.Update(w)
	if len(*opened) != 1 || !strings.Contains(a.toast.text, "no remote with a web page") {
		t.Fatalf("opened %v toast=%q", *opened, a.toast.text)
	}

	// Remote tab: the repo's GitHub page.
	a.remote.all = []provider.Repo{{Host: "github.com", Owner: "me", Name: "beta", WebURL: "https://github.com/me/beta"}}
	a.remote.refresh(a.localKeys())
	a.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	a.Update(w)
	if len(*opened) != 2 || (*opened)[1] != "https://github.com/me/beta" {
		t.Fatalf("opened %v", *opened)
	}
}

func TestWReportsBrowserErrors(t *testing.T) {
	old := openURL
	openURL = func(string) error { return errors.New("no browser") }
	t.Cleanup(func() { openURL = old })

	a := newDetailApp(t, 120, 24)
	a.local.selected().status.Remotes = []string{"https://github.com/me/alpha"}
	a.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if a.toast.kind != 2 || !strings.Contains(a.toast.text, "no browser") {
		t.Fatalf("toast = %d %q", a.toast.kind, a.toast.text)
	}
}

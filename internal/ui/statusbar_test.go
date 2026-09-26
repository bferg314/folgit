package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/gitinfo"
)

func barApp(t *testing.T, w int, powerline bool) *App {
	t.Helper()
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, "code")
	cfg := config.Default()
	cfg.Powerline = powerline
	a := New(cfg, filepath.Join(home, "folgit.toml"), root, nil, "")
	a.Update(tea.WindowSizeMsg{Width: w, Height: 12})
	for i, n := range []string{"plnngpkr", "folgit", "karst"} {
		r := a.local.add(filepath.Join(root, n), root, 0)
		r.status = &gitinfo.Status{Branch: "main", Upstream: "origin/main", LastCommit: time.Now().Add(-time.Duration(i) * time.Hour)}
		a.detail.cache[r.path] = &gitinfo.Details{}
	}
	a.local.rows[filepath.Join(root, "plnngpkr")].status.Unstaged = 1
	a.local.rows[filepath.Join(root, "karst")].status.Behind = 3
	a.local.refresh()
	return a
}

func bar(a *App) string {
	lines := strings.Split(a.render(), "\n")
	return lines[len(lines)-1]
}

func TestStatusBarContents(t *testing.T) {
	a := barApp(t, 140, false)
	b := ansi.Strip(bar(a))
	sep := string(filepath.Separator)
	for _, want := range []string{"LOCAL", "main ●1", "~" + sep + "code" + sep + "plnngpkr", "●1 dirty", "↓1 to pull", "1/3", "? help"} {
		if !strings.Contains(b, want) {
			t.Errorf("bar missing %q: %q", want, b)
		}
	}
	if strings.Contains(b, plRight) {
		t.Error("plain bar must not use powerline glyphs")
	}
	if !strings.Contains(ansi.Strip(bar(barApp(t, 140, true))), plRight) {
		t.Error("powerline bar should use arrow separators")
	}

	a.local.filter.Focus()
	if !strings.HasPrefix(strings.TrimSpace(ansi.Strip(bar(a))), "FILTER") {
		t.Errorf("focused filter should show FILTER mode: %q", ansi.Strip(bar(a)))
	}
	a.local.filter.Blur()
	a.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if !strings.HasPrefix(strings.TrimSpace(ansi.Strip(bar(a))), "REMOTE") {
		t.Errorf("remote tab should show REMOTE mode: %q", ansi.Strip(bar(a)))
	}
}

// The bar is always exactly the terminal width, keeps "? help", and never
// loses a message to less important segments.
func TestStatusBarFitsEveryWidth(t *testing.T) {
	for _, pl := range []bool{false, true} {
		for w := 30; w <= 200; w++ {
			a := barApp(t, w, pl)
			a.notify(2, "Pull karst failed: Not possible to fast-forward, aborting.")
			b := bar(a)
			if got := ansi.StringWidth(b); got != w {
				t.Fatalf("w=%d powerline=%v: bar is %d wide: %q", w, pl, got, ansi.Strip(b))
			}
			s := ansi.Strip(b)
			if !strings.Contains(s, "? help") || !strings.Contains(s, "✗ Pu") {
				t.Fatalf("w=%d powerline=%v: lost help or message: %q", w, pl, s)
			}
		}
	}
}

func TestTildePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	sep := string(filepath.Separator)
	if got := tildePath(filepath.Join(home, "code", "x")); got != "~"+sep+"code"+sep+"x" {
		t.Errorf("got %q", got)
	}
	if got := tildePath(home); got != "~" {
		t.Errorf("got %q", got)
	}
	other := filepath.Join(filepath.Dir(home), "someone-else")
	if got := tildePath(other); got != other {
		t.Errorf("paths outside home must not change: %q", got)
	}
}

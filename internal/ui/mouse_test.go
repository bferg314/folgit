package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bferg314/folgit/internal/provider"
	"github.com/bferg314/folgit/internal/provider/github"
)

// mouseAt sends a mouse event at (x, y) after drawing the screen, as the
// program does before any event can arrive.
func mouseAt(a *App, msg func(tea.Mouse) tea.Msg, x, y int, b tea.MouseButton) tea.Cmd {
	a.render()
	_, cmd := a.Update(msg(tea.Mouse{X: x, Y: y, Button: b}))
	return cmd
}

func click(a *App, x, y int) tea.Cmd {
	return mouseAt(a, func(m tea.Mouse) tea.Msg { return tea.MouseClickMsg(m) }, x, y, tea.MouseLeft)
}

func rightClick(a *App, x, y int) tea.Cmd {
	return mouseAt(a, func(m tea.Mouse) tea.Msg { return tea.MouseClickMsg(m) }, x, y, tea.MouseRight)
}

func wheel(a *App, x, y int, down bool) tea.Cmd {
	b := tea.MouseWheelUp
	if down {
		b = tea.MouseWheelDown
	}
	return mouseAt(a, func(m tea.Mouse) tea.Msg { return tea.MouseWheelMsg(m) }, x, y, b)
}

// find is where text first appears on the screen. The header shows the
// scanned folder, which holds the test's name, so a test's name mustn't
// contain the text it looks for.
func find(t *testing.T, a *App, text string) (x, y int) {
	t.Helper()
	for y, l := range strings.Split(screen(a), "\n") {
		if i := strings.Index(l, text); i >= 0 {
			return ansi.StringWidth(l[:i]), y
		}
	}
	t.Fatalf("%q isn't on the screen:\n%s", text, screen(a))
	return 0, 0
}

// clickText clicks the first place text appears, and forgets the click so
// the next one isn't a double click.
func clickText(t *testing.T, a *App, text string) tea.Cmd {
	t.Helper()
	x, y := find(t, a, text)
	cmd := click(a, x, y)
	a.mouse.lastAt = time.Time{}
	return cmd
}

func doubleClickText(t *testing.T, a *App, text string) tea.Cmd {
	t.Helper()
	x, y := find(t, a, text)
	click(a, x, y)
	cmd := click(a, x, y)
	a.mouse.lastAt = time.Time{}
	return cmd
}

func cursorRepo(a *App) string {
	if r := a.local.selected(); r != nil {
		return r.rel
	}
	return ""
}

func TestMouseTabs(t *testing.T) {
	a, _ := markApp(t)
	for i, name := range []string{"Remote", "Settings", "Local"} {
		clickText(t, a, name)
		if want := []int{tabRemote, tabSettings, tabLocal}[i]; a.tab != want {
			t.Fatalf("clicking %s: tab %d", name, a.tab)
		}
	}
}

func TestMouseLocalRows(t *testing.T) {
	a, _ := markApp(t)
	a.cfg.Tools[0].Enabled = true

	clickText(t, a, "middle")
	if cursorRepo(a) != "middle" || a.popup != nil {
		t.Fatalf("a click moves the cursor and nothing else: %s popup=%v", cursorRepo(a), a.popup != nil)
	}
	doubleClickText(t, a, "oldest")
	if cursorRepo(a) != "oldest" || a.popup == nil || a.popup.repo.rel != "oldest" {
		t.Fatalf("a double click opens the repo in a tool: %s popup=%v", cursorRepo(a), a.popup != nil)
	}

	// The popup is modal: a click on it that isn't on a tool does nothing,
	// even with a row behind it, and a click outside it closes it.
	clickText(t, a, "Open oldest")
	if a.popup == nil || cursorRepo(a) != "oldest" {
		t.Fatalf("a click on the popup's title: popup=%v cursor=%s", a.popup != nil, cursorRepo(a))
	}
	click(a, 0, a.h-2)
	if a.popup != nil {
		t.Fatal("a click outside the popup should close it")
	}

	x, y := find(t, a, "newest")
	rightClick(a, x, y)
	if cursorRepo(a) != "newest" || a.popup == nil {
		t.Fatalf("a right click opens the repo in a tool: %s popup=%v", cursorRepo(a), a.popup != nil)
	}
}

func TestMouseDoubleClickAfterLayoutChange(t *testing.T) {
	a, _ := markApp(t)
	a.cfg.Tools[0].Enabled = true
	press(a, "/") // an empty filter line, which goes when a row is clicked

	x, y := find(t, a, "middle")
	click(a, x, y)
	click(a, x, y) // now on the row below
	if a.popup != nil {
		t.Fatalf("the second click landed on another row and should only move: cursor %s", cursorRepo(a))
	}
	if cursorRepo(a) != "oldest" {
		t.Fatalf("cursor on %s, want the row now under the pointer", cursorRepo(a))
	}
}

func TestMouseSortHeadings(t *testing.T) {
	a, _ := markApp(t)
	clickText(t, a, "REPOSITORY")
	if a.local.sort != sortName || viewNames(a)[0] != "middle" {
		t.Fatalf("REPOSITORY sorts by name: %s %v", a.local.sort, viewNames(a))
	}
	clickText(t, a, "STATUS")
	if a.local.sort != sortDirty {
		t.Fatalf("STATUS sorts dirty first: %s", a.local.sort)
	}
	clickText(t, a, "COMMITTED")
	if a.local.sort != sortRecent || viewNames(a)[0] != "newest" {
		t.Fatalf("COMMITTED sorts by recent: %s %v", a.local.sort, viewNames(a))
	}
}

func TestMouseWheel(t *testing.T) {
	a := newDetailApp(t, 140, 20)
	wheel(a, 5, 5, true)
	if cursorRepo(a) != "beta" {
		t.Fatalf("the wheel over the list moves it: %s", cursorRepo(a))
	}
	wheel(a, 5, 5, false)
	if cursorRepo(a) != "alpha" {
		t.Fatalf("the wheel over the list moves it back: %s", cursorRepo(a))
	}

	wheel(a, a.w-5, 8, true)
	if a.detail.scroll != 3 || cursorRepo(a) != "alpha" {
		t.Fatalf("the wheel over the pane scrolls it: scroll %d cursor %s", a.detail.scroll, cursorRepo(a))
	}
	wheel(a, a.w-5, 8, false)
	if a.detail.scroll != 0 {
		t.Fatalf("scroll %d after scrolling back", a.detail.scroll)
	}
}

func TestMouseSettings(t *testing.T) {
	a, _ := markApp(t)
	a.Update(tea.WindowSizeMsg{Width: 160, Height: 60})
	a.setTab(tabSettings)

	clickText(t, a, "Powerline status bar")
	if a.cfg.Powerline {
		t.Fatal("a click on a setting's name only moves to it")
	}
	if it := a.settingItems()[a.settings.cursor]; it.label != "Powerline status bar" {
		t.Fatalf("cursor on %q", it.label)
	}
	x, y := find(t, a, "Powerline status bar")
	click(a, x-4, y) // its checkbox
	if !a.cfg.Powerline {
		t.Fatal("a click on the checkbox should change it")
	}
}

func TestMouseStatusBarHelp(t *testing.T) {
	a, _ := markApp(t)
	clickText(t, a, "? help")
	if !a.help {
		t.Fatal("? help in the status bar should open help")
	}
	click(a, 0, 0)
	if a.help {
		t.Fatal("a click outside help should close it")
	}
}

func TestMouseRemoteSelect(t *testing.T) {
	a, _ := markApp(t)
	a.remote.all = []provider.Repo{
		{Owner: "me", Name: "one", Mine: true, PushedAt: time.Now()},
		{Owner: "me", Name: "two", Mine: true, PushedAt: time.Now().Add(-time.Hour)},
	}
	a.remote.refresh(a.localKeys())
	a.setTab(tabRemote)

	x, y := find(t, a, "me/two")
	click(a, x-2, y) // its ○
	a.mouse.lastAt = time.Time{}
	if !a.remote.selected[a.remote.view[1].Key()] || a.remote.cursor != 1 {
		t.Fatalf("a click on ○ selects the repo: %v cursor %d", a.remote.selected, a.remote.cursor)
	}
	clickText(t, a, "me/one")
	if a.remote.cursor != 0 || a.remote.selected[a.remote.view[0].Key()] {
		t.Fatalf("a click on the name only moves: cursor %d %v", a.remote.cursor, a.remote.selected)
	}
}

func TestMouseIssueForm(t *testing.T) {
	a, _ := markApp(t)
	a.issues = &issuesView{}
	a.Update(issuesMsg{view: a.issues, repo: "me/alpha", issues: []github.Issue{
		{Number: 1, Title: "first"}, {Number: 2, Title: "second"},
	}})
	clickText(t, a, "second")
	if a.issues.cursor != 1 {
		t.Fatalf("a click moves to the issue: cursor %d", a.issues.cursor)
	}

	run(a, a.openIssueForm())
	clickText(t, a, "Description")
	if !a.issues.form.onBody {
		t.Fatal("a click on the description should move to it")
	}
	click(a, 0, 0)
	if a.issues == nil || !a.issues.form.open {
		t.Fatal("a click outside the form mustn't close it")
	}
}

func TestMouseOff(t *testing.T) {
	a, _ := markApp(t)
	a.cfg.Mouse = false
	clickText(t, a, "Remote")
	if a.tab != tabLocal {
		t.Fatal("with the mouse off, clicks do nothing")
	}
	if a.View().MouseMode != tea.MouseModeNone {
		t.Fatal("with the mouse off, folgit shouldn't ask for mouse events")
	}
}

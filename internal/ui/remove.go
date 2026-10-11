package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/trash"
)

// removeView is the popup that deletes a repo's folder. It checks what
// would be lost, asks for the folder's name to be typed, checks again just
// before deleting, and prefers the trash to a permanent delete.
type removeView struct {
	path, rel, name string
	loading         bool
	removing        bool
	check           gitinfo.Removal
	trashDir        string // "" when the folder would be deleted permanently
	changed         bool   // the recheck found something new
	err             error
	input           textinput.Model
}

type (
	removeCheckMsg struct {
		view     *removeView
		check    gitinfo.Removal
		trashDir string
	}
	removeDoneMsg struct {
		view   *removeView
		check  gitinfo.Removal // the recheck, if the risks changed
		dest   string          // where it went in the trash; "" if deleted
		err    error
		halted bool // the recheck differed, so nothing was deleted
	}
)

// openRemove starts the delete popup for the selected repo.
func (a *App) openRemove() tea.Cmd {
	r := a.local.selected()
	switch {
	case r == nil:
		return nil
	case r.busy != "" || a.bulk != nil:
		return a.notify(2, "Wait for the pull or fetch to finish first")
	}
	v := &removeView{path: r.path, rel: r.rel, name: filepath.Base(r.path), loading: true}
	v.input = textinput.New()
	v.input.Prompt = "│ "
	ts := v.input.Styles()
	ts.Focused.Prompt, ts.Blurred.Prompt = a.st.bad, a.st.faintText
	ts.Focused.Text, ts.Blurred.Text = a.st.textS, a.st.textS
	ts.Cursor.Blink = false
	v.input.SetStyles(ts)
	a.remove = v
	return tea.Batch(a.startSpinner(), a.checkRemove(v))
}

func (a *App) checkRemove(v *removeView) tea.Cmd {
	root := a.root
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return removeCheckMsg{view: v, check: gitinfo.CheckRemoval(ctx, root, v.path), trashDir: trash.Dir(v.path)}
	}
}

func (a *App) handleRemoveCheck(msg removeCheckMsg) tea.Cmd {
	v := a.remove
	if v == nil || v != msg.view {
		return nil
	}
	v.loading = false
	v.check, v.trashDir = msg.check, msg.trashDir
	if v.check.Blocked != "" {
		return nil
	}
	return v.input.Focus()
}

// confirmRemove checks the folder again, so nothing that appeared since
// the popup opened is lost unseen, then trashes or deletes it.
func (a *App) confirmRemove() tea.Cmd {
	v := a.remove
	v.removing, v.err = true, nil
	root, seen, toTrash := a.root, v.check, v.trashDir != ""
	return tea.Batch(a.startSpinner(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		now := gitinfo.CheckRemoval(ctx, root, v.path)
		if !now.SameRisks(seen) || toTrash != (trash.Dir(v.path) != "") {
			return removeDoneMsg{view: v, check: now, halted: true}
		}
		if toTrash {
			dest, err := trash.Move(v.path)
			return removeDoneMsg{view: v, dest: dest, err: err}
		}
		return removeDoneMsg{view: v, err: os.RemoveAll(v.path)}
	})
}

func (a *App) handleRemoveDone(msg removeDoneMsg) tea.Cmd {
	v := msg.view
	v.removing = false
	switch {
	case msg.halted:
		v.check, v.changed = msg.check, true
		v.trashDir = trash.Dir(v.path)
		v.input.SetValue("")
		if v.check.Blocked != "" {
			v.input.Blur()
		}
		return nil
	case msg.err != nil:
		v.err = msg.err
		// A failed permanent delete can leave part of the folder behind.
		return refreshStatus(v.path)
	}

	if a.remove == v {
		a.remove = nil
	}
	delete(a.local.rows, v.path)
	a.invalidateDetails(v.path)
	a.cfg.SetPinned(v.path, false)
	a.cfg.SetHidden(v.path, false)
	a.refreshViews()
	note := a.notify(1, "Deleted %s", v.rel)
	if msg.dest != "" {
		note = a.notify(1, "Moved %s to the trash: %s", v.rel, tildePath(msg.dest))
	}
	return tea.Batch(a.saveConfig(), a.saveCache(), note)
}

func (a *App) removeKey(msg tea.KeyPressMsg) tea.Cmd {
	v := a.remove
	if v.removing {
		return nil
	}
	key := msg.String()
	if key == "esc" || v.loading || v.check.Blocked != "" {
		if key == "esc" || key == "q" || key == "enter" {
			a.remove = nil
		}
		return nil
	}
	if key == "enter" {
		if v.input.Value() == v.name {
			return a.confirmRemove()
		}
		return nil
	}
	var cmd tea.Cmd
	v.input, cmd = v.input.Update(msg)
	return cmd
}

// updateRemoveInput passes non-key messages (pastes, cursor) to the input.
func (a *App) updateRemoveInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	a.remove.input, cmd = a.remove.input.Update(msg)
	return cmd
}

func (a *App) renderRemove() string {
	st := a.st
	v := a.remove
	cw := max(36, min(a.w-4, 84)) - 2 - 4

	lines := []string{
		fit(st.bad.Render("Delete folder")+st.dim.Render(" · ")+st.bold.Render(v.rel), cw),
		fit(st.dim.Render(tildePath(v.path)), cw),
		"",
	}
	if v.loading {
		lines = append(lines, a.spin.View()+st.dim.Render(" Checking for work that would be lost…"))
		return st.box.BorderForeground(st.red).Render(strings.Join(lines, "\n"))
	}

	c := v.check
	if v.changed {
		lines = append(lines, fit(st.warn.Render("Something changed since the last check, so nothing was deleted. Review it and confirm again."), cw), "")
	}
	if c.Blocked != "" {
		lines = append(lines,
			fit(st.bad.Render("✗ ")+st.textS.Render("Can't delete this folder: "+c.Blocked+"."), cw),
			"", st.key.Render("esc")+st.dim.Render(" close"))
		return st.box.BorderForeground(st.red).Render(strings.Join(lines, "\n"))
	}

	good := func(s string) string { return fit(st.ok.Render("✓ ")+st.dim.Render(s), cw) }
	bad := func(s string) string { return fit(st.bad.Render("✗ ")+st.textS.Render(s), cw) }
	if c.Changes > 0 {
		lines = append(lines, bad(fmt.Sprintf("%d uncommitted %s, including untracked files", c.Changes, plural(c.Changes, "change", "changes"))))
	} else {
		lines = append(lines, good("No uncommitted changes"))
	}
	switch {
	case c.NoRemotes && c.Unpushed > 0:
		lines = append(lines, bad(fmt.Sprintf("No remotes: its %d %s exist only here", c.Unpushed, plural(c.Unpushed, "commit", "commits"))))
	case c.NoRemotes:
		lines = append(lines, bad("No remotes: nothing in it exists anywhere else"))
	case c.Unpushed > 0:
		lines = append(lines, bad(fmt.Sprintf("%d %s on local branches or tags that aren't on any remote", c.Unpushed, plural(c.Unpushed, "commit", "commits"))))
	default:
		lines = append(lines, good("Every commit is on a remote"))
	}
	if c.Stashes > 0 {
		lines = append(lines, bad(fmt.Sprintf("%d %s", c.Stashes, plural(c.Stashes, "stash", "stashes"))))
	} else {
		lines = append(lines, good("No stashes"))
	}
	if n := len(c.Nested); n > 0 {
		lines = append(lines, bad(fmt.Sprintf("Contains %d other %s, not checked: %s", n, plural(n, "repo", "repos"), sample(c.Nested, 3))))
	}
	if n := len(c.Ignored); n > 0 {
		lines = append(lines, fit(st.warn.Render("! ")+st.textS.Render(fmt.Sprintf("%d ignored %s, which git never pushes: ", n, plural(n, "path", "paths")))+
			st.dim.Render(sample(c.Ignored, 4)), cw))
	}

	lines = append(lines, "")
	if v.trashDir != "" {
		lines = append(lines, fit(st.dim.Render("The folder goes to the trash ("+tildePath(v.trashDir)+"), so you can restore it."), cw))
	} else {
		lines = append(lines, fit(st.bad.Render("There's no trash here: the folder will be deleted permanently."), cw))
	}
	if v.err != nil {
		lines = append(lines, fit(st.bad.Render("Failed: "+v.err.Error()), cw))
	}

	lines = append(lines, "", fit(st.textS.Render("Type ")+st.bold.Render(v.name)+st.textS.Render(" to confirm:"), cw))
	v.input.SetWidth(cw - 2)
	lines = append(lines, v.input.View(), "")

	var hint string
	switch {
	case v.removing:
		hint = a.spin.View() + st.dim.Render(" Checking again and deleting…")
	case v.input.Value() == v.name:
		verb := "move to trash"
		if v.trashDir == "" {
			verb = "delete permanently"
		}
		hint = st.key.Render("enter") + st.dim.Render(" "+verb+" · ") + st.key.Render("esc") + st.dim.Render(" cancel")
	default:
		hint = st.key.Render("esc") + st.dim.Render(" cancel")
	}
	lines = append(lines, fit(hint, cw))
	return st.box.BorderForeground(st.red).Render(strings.Join(lines, "\n"))
}

// sample joins the first n items, noting how many more there are.
func sample(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:n], ", ") + fmt.Sprintf(", +%d more", len(items)-n)
}

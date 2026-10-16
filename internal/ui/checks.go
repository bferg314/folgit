package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/provider/github"
)

// checksView is the popup listing the selected repo's checks, each with a
// fix that enter runs.
type checksView struct {
	path, rel string
	cursor    int
	fixing    bool
	err       error
}

type (
	protocolMsg struct{ protocol, from string }
	fixDoneMsg  struct {
		view *checksView
		note string
		err  error
	}
)

// resolveProtocol looks up the clone protocol, which decides whether HTTPS
// remotes are flagged.
func (a *App) resolveProtocol() tea.Cmd {
	configured := a.cfg.GitHub.Protocol
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		p, from := github.ResolveProtocol(ctx, configured)
		return protocolMsg{protocol: p, from: from}
	}
}

// repoCheck is one check on a repo.
type repoCheck struct {
	name string
	ok   bool
	// problem is a short description of what's wrong, for the list and
	// detail pane; fix lists what enter does about it.
	problem string
	fix     []string
	run     func(ctx context.Context, path string) (string, error)
}

// repoChecks runs the checks against what's known about r. HTTPS remotes
// are known from the scan; line endings once the repo's details are loaded.
// known is false until then.
func (a *App) repoChecks(r *localRepo) (checks []repoCheck, known bool) {
	d := a.detail.cache[r.path]
	if a.protocol == "ssh" {
		if c, ok := a.sshCheck(r, d); ok {
			checks = append(checks, c)
		}
	}
	if d != nil && d.LineEndings != nil {
		checks = append(checks, lineEndingsCheck(*d.LineEndings, r.status))
	}
	return checks, d != nil
}

// failing reports whether any known check on r fails.
func (a *App) failing(r *localRepo) bool {
	cs, _ := a.repoChecks(r)
	for _, c := range cs {
		if !c.ok {
			return true
		}
	}
	return false
}

func (a *App) sshCheck(r *localRepo, d *gitinfo.Details) (repoCheck, bool) {
	c := repoCheck{name: "Remotes use SSH", ok: true}
	if d != nil {
		if len(d.HTTPS) == 0 {
			return c, true
		}
		c.ok = false
		names := make([]string, len(d.HTTPS))
		for i, h := range d.HTTPS {
			names[i] = h.Name
			c.fix = append(c.fix, fmt.Sprintf("%s: %s → %s", h.Name, h.URL, h.SSHURL))
		}
		c.problem = strings.Join(names, ", ") + " " + plural(len(names), "uses", "use") + " HTTPS"
		rs := d.HTTPS
		c.run = func(ctx context.Context, path string) (string, error) {
			if err := gitinfo.UseSSH(ctx, path, rs); err != nil {
				return "", err
			}
			return fmt.Sprintf("Switched %s to SSH", strings.Join(names, ", ")), nil
		}
		return c, true
	}
	// Until the details load, the scan's remote URLs are all there is.
	if r.status == nil {
		return c, false
	}
	n := 0
	for _, u := range r.status.Remotes {
		if _, ok := gitinfo.SSHURL(u); ok {
			n++
		}
	}
	if n == 0 {
		return c, true
	}
	c.ok = false
	c.problem = fmt.Sprintf("%d %s HTTPS", n, plural(n, "remote uses", "remotes use"))
	return c, true
}

func lineEndingsCheck(l gitinfo.LineEndings, s *gitinfo.Status) repoCheck {
	c := repoCheck{name: "Line endings", ok: l.OK()}
	if c.ok {
		return c
	}
	var parts []string
	if n := len(l.Committed); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s committed with CRLF", n, plural(n, "file", "files")))
	}
	if n := len(l.CheckedOut); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s checked out with CRLF", n, plural(n, "file", "files")))
	}
	c.problem = strings.Join(parts, ", ")
	if !l.Attributes {
		c.fix = append(c.fix, "adds .gitattributes: * text=auto eol=lf")
	}
	if n := len(l.Committed); n > 0 {
		c.fix = append(c.fix, fmt.Sprintf("restages %d %s with LF, for you to review and commit", n, plural(n, "file", "files")))
	}
	c.fix = append(c.fix, "rewrites files checked out with CRLF as LF (only line endings change)")
	if s != nil && s.Staged+s.Unstaged+s.Conflicts > 0 {
		c.fix = append(c.fix, "! needs no uncommitted changes: commit or stash them first")
	}
	c.run = func(ctx context.Context, path string) (string, error) {
		f, err := gitinfo.FixLineEndings(ctx, path)
		if err != nil {
			return "", err
		}
		var did []string
		if f.AddedAttributes {
			did = append(did, "added .gitattributes")
		}
		if f.Renormalized > 0 {
			did = append(did, fmt.Sprintf("staged %d %s with LF", f.Renormalized, plural(f.Renormalized, "file", "files")))
		}
		if f.Rewritten > 0 {
			did = append(did, fmt.Sprintf("rewrote %d %s", f.Rewritten, plural(f.Rewritten, "file", "files")))
		}
		if len(did) == 0 {
			return "Line endings were already fine", nil
		}
		return "Line endings: " + strings.Join(did, ", "), nil
	}
	return c
}

// openChecks shows the selected repo's checks, loading its details first
// if the detail pane hasn't.
func (a *App) openChecks() tea.Cmd {
	r := a.local.selected()
	if r == nil {
		return nil
	}
	a.checks = &checksView{path: r.path, rel: r.rel}
	if a.detail.cache[r.path] == nil {
		return tea.Batch(loadDetails(r.path), a.startSpinner())
	}
	return nil
}

// checksRepo is the repo the popup is for, if it's still listed.
func (a *App) checksRepo() *localRepo { return a.local.rows[a.checks.path] }

func (a *App) checksKey(key string) tea.Cmd {
	v := a.checks
	if v.fixing {
		return nil
	}
	r := a.checksRepo()
	if r == nil {
		a.checks = nil
		return nil
	}
	cs, _ := a.repoChecks(r)
	switch key {
	case "esc", "q", "C":
		a.checks = nil
	case "up", "k":
		v.cursor = max(0, v.cursor-1)
	case "down", "j":
		v.cursor = min(max(0, len(cs)-1), v.cursor+1)
	case "enter":
		if v.cursor >= len(cs) || cs[v.cursor].ok || cs[v.cursor].run == nil {
			return nil
		}
		v.fixing, v.err = true, nil
		run, path := cs[v.cursor].run, v.path
		return tea.Batch(a.startSpinner(), func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			note, err := run(ctx, path)
			return fixDoneMsg{view: v, note: note, err: err}
		})
	}
	return nil
}

func (a *App) handleFixDone(msg fixDoneMsg) tea.Cmd {
	v := msg.view
	v.fixing = false
	// Reload whatever the fix changed, whether or not it got all the way.
	a.invalidateDetails(v.path)
	cmds := []tea.Cmd{refreshStatus(v.path), loadDetails(v.path), a.startSpinner()}
	if msg.err != nil {
		v.err = msg.err
		if errors.Is(msg.err, gitinfo.ErrUncommitted) {
			v.err = errors.New("there are uncommitted changes: commit or stash them first")
		}
		return tea.Batch(cmds...)
	}
	return tea.Batch(append(cmds, a.notify(1, "%s: %s", v.rel, msg.note))...)
}

func (a *App) renderChecks() string {
	st := a.st
	v := a.checks
	bw, _ := a.popupBoxSize()
	bw = min(bw, 96)
	cw := bw - 2 - 4

	title := st.boxTitle.Render("Repo checks") + st.dim.Render(" · ") + st.bold.Render(v.rel)
	var right string
	if v.fixing || a.detail.cache[v.path] == nil {
		right = a.spin.View()
	}
	gap := max(1, cw-lipgloss.Width(title)-lipgloss.Width(right))
	lines := []string{fit(title+strings.Repeat(" ", gap)+right, cw), ""}

	r := a.checksRepo()
	var cs []repoCheck
	known := false
	if r != nil {
		cs, known = a.repoChecks(r)
	}
	v.cursor = min(v.cursor, max(0, len(cs)-1))
	for i, c := range cs {
		marker := "  "
		name := st.textS.Render(c.name)
		if i == v.cursor {
			marker = st.marker.Render("▌") + " "
			name = st.selName.Render(c.name)
		}
		mark, note := st.ok.Render("✓ "), ""
		if !c.ok {
			mark, note = st.bad.Render("✗ "), st.warn.Render(c.problem)
		}
		lines = append(lines, marker+mark+fit(name, 18)+" "+fit(note, max(1, cw-22)))
		if !c.ok && i == v.cursor {
			for _, f := range c.fix {
				if rest, ok := strings.CutPrefix(f, "! "); ok {
					lines = append(lines, "      "+fit(st.warn.Render(rest), cw-6))
				} else {
					lines = append(lines, "      "+fit(st.dim.Render("· "+f), cw-6))
				}
			}
		}
	}
	switch {
	case !known:
		lines = append(lines, "  "+a.spin.View()+st.dim.Render(" checking line endings…"))
	case a.protocol != "ssh":
		lines = append(lines, "", fit(st.dim.Render("HTTPS remotes aren't flagged: clones use HTTPS. Change it in Settings."), cw))
	}
	if v.err != nil {
		lines = append(lines, "", fit(st.bad.Render("✗ ")+st.dim.Render(v.err.Error()), cw))
	}

	hint := st.key.Render("enter") + st.dim.Render(" fix the highlighted check · ") + st.key.Render("esc") + st.dim.Render(" close")
	lines = append(lines, "", fit(hint, cw))
	return st.box.Width(bw).Render(strings.Join(lines, "\n"))
}

// appendChecks adds the failing checks to the detail pane.
func (a *App) appendChecks(r *localRepo, add func(string), section func(string)) {
	cs, _ := a.repoChecks(r)
	var bad []repoCheck
	for _, c := range cs {
		if !c.ok {
			bad = append(bad, c)
		}
	}
	if len(bad) == 0 {
		return
	}
	section("CHECKS")
	for _, c := range bad {
		add(a.st.bad.Render("✗ ") + a.st.textS.Render(c.name+": ") + a.st.warn.Render(c.problem))
	}
	add(a.st.dim.Render("C to fix"))
}

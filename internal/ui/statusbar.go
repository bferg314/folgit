package ui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Powerline glyphs (need a Nerd Font / Powerline font).
const (
	plRight     = "" // solid arrow pointing right
	plRightThin = ""
	plLeft      = "" // solid arrow pointing left
	plLeftThin  = ""
	plBranch    = ""
)

// part is a run of text inside a segment. Every part is rendered with the
// segment's background, since a nested style's reset would otherwise clear
// it mid-segment.
type part struct {
	text string
	fg   color.Color
	bold bool
}

type segment struct {
	parts []part
	bg    color.Color
}

func seg(bg color.Color, parts ...part) segment { return segment{parts: parts, bg: bg} }

func (s segment) empty() bool {
	for _, p := range s.parts {
		if p.text != "" {
			return false
		}
	}
	return true
}

func (s segment) width() int {
	w := 2 // padding
	for _, p := range s.parts {
		w += ansi.StringWidth(p.text)
	}
	return w
}

func (s segment) render() string {
	pad := lipgloss.NewStyle().Background(s.bg).Render(" ")
	var b strings.Builder
	b.WriteString(pad)
	for _, p := range s.parts {
		b.WriteString(lipgloss.NewStyle().Background(s.bg).Foreground(p.fg).Bold(p.bold).Render(p.text))
	}
	b.WriteString(pad)
	return b.String()
}

func sameColor(a, b color.Color) bool {
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

// renderStatusBar draws the airline-style bar at the bottom of the screen:
// mode, branch and path on the left; activity, a summary across all repos,
// position and a help hint on the right.
func (a *App) renderStatusBar() string {
	st := a.st
	mode, modeBg := a.barMode()
	left := []segment{seg(modeBg, part{text: mode, fg: st.accentFg, bold: true})}
	left = append(left, a.barContext()...)
	path := a.barPath()

	activity, summary, position := a.barActivity(), a.barSummary(), a.barPosition()
	help := seg(modeBg, part{text: "? help", fg: st.accentFg, bold: true})
	right := func() []segment {
		var r []segment
		for _, s := range []segment{activity, summary, position, help} {
			if !s.empty() {
				r = append(r, s)
			}
		}
		return r
	}

	// Fit, giving things up in order of importance: shorten the path from
	// the left (the repo folder is at the end) down to a minimum, drop the
	// summary, drop the position, shorten the message, and finally the path.
	const minPath = 12
	room := func() int { return a.w - a.barWidth(left, right()) - 3 } // path padding and trailing space
	if room() < minPath && !summary.empty() {
		summary = segment{}
	}
	if room() < minPath && !position.empty() {
		position = segment{}
	}
	if over := minPath - room(); over > 0 && !activity.empty() {
		activity = activity.shorten(over)
	}
	if room() < 0 && len(left) > 1 {
		left = left[:1] // very narrow: keep only the mode on the left
	}
	switch r := room(); {
	case r >= ansi.StringWidth(path):
	case r >= 2:
		path = "…" + ansi.TruncateLeft(path, ansi.StringWidth(path)-r+1, "")
	default:
		path = ""
	}
	left = append(left, seg(st.barLow, part{text: path, fg: st.text}))
	return a.joinBar(left, right())
}

// shorten trims n cells off the end of the segment's text (last part
// first), keeping at least a few characters and marking the cut with "…".
func (s segment) shorten(n int) segment {
	parts := append([]part{}, s.parts...)
	for i := len(parts) - 1; i >= 0 && n > 0; i-- {
		w := ansi.StringWidth(parts[i].text)
		newW := max(5, w-n) // ansi.Truncate counts the "…" in newW
		if newW >= w {
			continue
		}
		parts[i].text = ansi.Truncate(parts[i].text, newW, "…")
		n -= w - newW
	}
	s.parts = parts
	return s
}

// barWidth is the width of all segments and separators, excluding the
// path segment's text.
func (a *App) barWidth(left, right []segment) int {
	w := len(left) + len(right) // one separator after each left and before each right segment
	for _, s := range append(append([]segment{}, left...), right...) {
		w += s.width()
	}
	return w
}

func (a *App) joinBar(left, right []segment) string {
	st := a.st
	var b strings.Builder
	sep := func(glyph string, fg, bg color.Color) {
		if a.cfg.Powerline {
			b.WriteString(lipgloss.NewStyle().Foreground(fg).Background(bg).Render(glyph))
		} else {
			b.WriteString(lipgloss.NewStyle().Background(bg).Render(" "))
		}
	}

	for i, s := range left {
		b.WriteString(s.render())
		next := st.barLow
		if i+1 < len(left) {
			next = left[i+1].bg
		}
		switch {
		case i == len(left)-1:
			// The path segment runs into the fill; no separator needed.
			b.WriteString(lipgloss.NewStyle().Background(s.bg).Render(" "))
		case sameColor(s.bg, next):
			sep(plRightThin, st.muted, s.bg)
		default:
			sep(plRight, s.bg, next)
		}
	}

	var rb strings.Builder
	prev := st.barLow
	for _, s := range right {
		if a.cfg.Powerline {
			glyph, fg := plLeft, s.bg
			if sameColor(prev, s.bg) {
				glyph, fg = plLeftThin, st.muted
			}
			rb.WriteString(lipgloss.NewStyle().Foreground(fg).Background(prev).Render(glyph))
		} else {
			rb.WriteString(lipgloss.NewStyle().Background(prev).Render(" "))
		}
		rb.WriteString(s.render())
		prev = s.bg
	}

	fill := a.w - lipgloss.Width(b.String()) - lipgloss.Width(rb.String())
	if fill > 0 {
		b.WriteString(lipgloss.NewStyle().Background(st.barLow).Render(strings.Repeat(" ", fill)))
	}
	b.WriteString(rb.String())
	return ansi.Truncate(b.String(), a.w, "")
}

// barMode names what the keyboard is currently driving, like vim's modes.
func (a *App) barMode() (string, color.Color) {
	st := a.st
	switch {
	case a.help:
		return "HELP", st.accent
	case a.cleanup != nil:
		return "BRANCHES", st.green
	case a.issues != nil:
		return "ISSUES", st.green
	case a.popup != nil:
		return "OPEN", st.green
	}
	if f := a.activeFilter(); f != nil && f.Focused() {
		return "FILTER", st.yellow
	}
	switch a.tab {
	case tabRemote:
		return "REMOTE", st.blue
	case tabSettings:
		return "SETTINGS", st.magenta
	}
	return "LOCAL", st.accent
}

// barContext is the segment after the mode: the selected repo's branch and
// state, or a remote repo's language and stars.
func (a *App) barContext() []segment {
	st := a.st
	var parts []part
	switch a.tab {
	case tabLocal:
		r := a.local.selected()
		if r == nil || r.status == nil || r.status.Err != nil {
			return nil
		}
		s := r.status
		icon := ""
		if a.cfg.Powerline {
			icon = plBranch + " "
		}
		parts = append(parts, part{text: icon + branchLabel(s), fg: st.text, bold: true})
		add := func(n int, format string, fg color.Color) {
			if n > 0 {
				parts = append(parts, part{text: " " + fmt.Sprintf(format, n), fg: fg})
			}
		}
		add(s.Conflicts, "!%d", st.red)
		add(s.Staged+s.Unstaged+s.Untracked, "●%d", st.yellow)
		add(s.Ahead, "↑%d", st.blue)
		add(s.Behind, "↓%d", st.magenta)
		add(s.Stashes, "≡%d", st.muted)
	case tabRemote:
		r := a.remote.current()
		if r == nil {
			return nil
		}
		if r.Language != "" {
			parts = append(parts, part{text: r.Language, fg: st.text, bold: true})
		}
		if r.Stars > 0 {
			parts = append(parts, part{text: fmt.Sprintf(" ★%d", r.Stars), fg: st.yellow})
		}
		if r.Private {
			parts = append(parts, part{text: " private", fg: st.muted})
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return []segment{seg(st.barMid, parts...)}
}

func (a *App) barPath() string {
	switch a.tab {
	case tabLocal:
		if r := a.local.selected(); r != nil {
			return tildePath(r.path)
		}
		return tildePath(a.root)
	case tabRemote:
		if r := a.remote.current(); r != nil {
			return strings.TrimPrefix(r.WebURL, "https://")
		}
	case tabSettings:
		return tildePath(a.cfgPath)
	}
	return ""
}

// barActivity shows the latest message, or what folgit is busy with.
func (a *App) barActivity() segment {
	st := a.st
	if a.toast.text != "" {
		bg := [...]color.Color{st.accent, st.green, st.red}[a.toast.kind]
		icon := [...]string{"•", "✓", "✗"}[a.toast.kind]
		return seg(bg, part{text: icon + " " + a.toast.text, fg: st.accentFg, bold: true})
	}
	var what string
	switch {
	case a.bulk != nil:
		what = a.bulkProgress()
	case a.local.scanning:
		what = "scanning"
	case len(a.remote.cloning) > 0:
		what = fmt.Sprintf("cloning %d", len(a.remote.cloning))
	case a.local.anyBusy():
		what = "pulling"
	case a.remote.loading:
		what = "loading GitHub"
	}
	if what == "" {
		return segment{}
	}
	return seg(st.barMid, part{text: a.spin.View(), fg: st.accent}, part{text: " " + what, fg: st.text})
}

// barSummary totals the state of every local repo that isn't hidden.
func (a *App) barSummary() segment {
	st := a.st
	var dirty, ahead, behind, conflicts int
	for _, r := range a.local.rows {
		if r.hidden || r.status == nil || r.status.Err != nil {
			continue
		}
		if r.status.Conflicts > 0 {
			conflicts++
		}
		if r.status.Dirty() {
			dirty++
		}
		if r.status.Ahead > 0 {
			ahead++
		}
		if r.status.Behind > 0 {
			behind++
		}
	}
	var parts []part
	add := func(n int, format string, fg color.Color) {
		if n > 0 {
			if len(parts) > 0 {
				parts = append(parts, part{text: " ", fg: st.text})
			}
			parts = append(parts, part{text: fmt.Sprintf(format, n), fg: fg})
		}
	}
	add(conflicts, "!%d", st.red)
	add(dirty, "●%d dirty", st.yellow)
	add(ahead, "↑%d to push", st.blue)
	add(behind, "↓%d to pull", st.magenta)
	if len(parts) == 0 {
		if len(a.local.rows) == a.local.hiddenCount() {
			return segment{}
		}
		parts = append(parts, part{text: "✓ all clean", fg: st.green})
	}
	return seg(st.barLow, parts...)
}

func (a *App) barPosition() segment {
	var i, n int
	switch a.tab {
	case tabLocal:
		i, n = a.local.cursor, len(a.local.view)
	case tabRemote:
		i, n = a.remote.cursor, len(a.remote.view)
	}
	if n == 0 {
		return segment{}
	}
	return seg(a.st.barMid, part{text: fmt.Sprintf("%d/%d", i+1, n), fg: a.st.text, bold: true})
}

// tildePath shortens the home directory to ~.
func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		if rel == "." {
			return "~"
		}
		return "~" + string(filepath.Separator) + rel
	}
	return p
}

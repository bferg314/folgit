package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// fit pads or truncates a styled string to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

// ago formats t relative to now: "just now", "5m ago", "3d ago", ...
func ago(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/24/365))
	}
}

// agoStyle makes recent activity stand out and old activity recede.
func (s styles) agoStyle(t time.Time) lipgloss.Style {
	switch d := time.Since(t); {
	case t.IsZero():
		return s.faintText
	case d < 24*time.Hour:
		return s.ok
	case d < 30*24*time.Hour:
		return s.textS
	default:
		return s.dim
	}
}

// highlightName renders a path-like name in w cells: the directory part is
// muted, the base name is normal (or accent when selected), and fuzzy-matched
// characters are highlighted. Long names are truncated from the left so the
// base name stays visible. matches are byte offsets into name.
func (s styles) highlightName(name string, w int, matches []int, selected bool) string {
	runes := []rune(name)
	matched := make(map[int]bool, len(matches))
	for _, b := range matches {
		matched[utf8.RuneCountInString(name[:b])] = true
	}
	baseStart := strings.LastIndex(name, "/") + 1
	baseStart = utf8.RuneCountInString(name[:baseStart])

	offset, prefix := 0, ""
	if len(runes) > w && w > 1 {
		offset = len(runes) - (w - 1)
		prefix = s.dim.Render("…")
	}

	base := s.textS
	if selected {
		base = s.selName
	}
	styleFor := [...]lipgloss.Style{base, s.dim, s.match}
	category := func(i int) int {
		switch {
		case matched[i]:
			return 2
		case i < baseStart:
			return 1
		}
		return 0
	}

	var b strings.Builder
	b.WriteString(prefix)
	start := offset
	for i := offset; i <= len(runes); i++ {
		if i == len(runes) || category(i) != category(start) {
			if i > start {
				b.WriteString(styleFor[category(start)].Render(string(runes[start:i])))
			}
			start = i
		}
	}
	return fit(b.String(), w)
}

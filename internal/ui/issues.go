package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/launcher"
	"github.com/bferg314/folgit/internal/match"
	"github.com/bferg314/folgit/internal/provider/github"
)

// The GitHub client is created once (finding a token can mean running
// `gh auth token`) and rebuilt only when the GitHub settings change.
var (
	ghMu     sync.Mutex
	ghClient *github.GitHub
	ghOpt    config.GitHub
)

func githubClient(ctx context.Context, opt config.GitHub) (*github.GitHub, error) {
	ghMu.Lock()
	defer ghMu.Unlock()
	if ghClient != nil && ghOpt == opt {
		return ghClient, nil
	}
	g, err := github.New(ctx, opt)
	if err != nil {
		return nil, err
	}
	ghClient, ghOpt = g, opt
	return g, nil
}

// issuesView is the popup listing a repo's open GitHub issues.
type issuesView struct {
	name    string   // shown in the title
	repos   []string // candidate "owner/repo", tried in order
	repo    string   // the one that answered
	loading bool
	err     error
	issues  []github.Issue
	more    bool
	cursor  int
	offset  int
}

type issuesMsg struct {
	view   *issuesView // identifies the request; stale replies are dropped
	repo   string
	issues []github.Issue
	more   bool
	err    error
}

// openIssues shows the issues popup for a repo with the given GitHub
// remotes ("owner/repo").
func (a *App) openIssues(name string, repos []string) tea.Cmd {
	if len(repos) == 0 {
		return a.notify(2, "%s has no GitHub remote", name)
	}
	a.issues = &issuesView{name: name, repos: repos}
	return a.loadIssues()
}

func (a *App) loadIssues() tea.Cmd {
	v := a.issues
	v.loading, v.err = true, nil
	opt := a.cfg.GitHub
	return tea.Batch(a.startSpinner(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		gh, err := githubClient(ctx, opt)
		if err != nil {
			return issuesMsg{view: v, err: err}
		}
		// A fork usually has issues turned off; fall through to the next
		// GitHub remote (typically upstream) on a 404.
		for _, repo := range v.repos {
			owner, name, _ := strings.Cut(repo, "/")
			issues, more, err := gh.Issues(ctx, owner, name)
			if errors.Is(err, github.ErrNotFound) {
				continue
			}
			return issuesMsg{view: v, repo: repo, issues: issues, more: more, err: err}
		}
		return issuesMsg{view: v, err: github.ErrNotFound}
	})
}

func (a *App) handleIssues(msg issuesMsg) {
	v := a.issues
	if v == nil || v != msg.view {
		return
	}
	v.loading = false
	v.err = msg.err
	v.repo = msg.repo
	v.issues, v.more = msg.issues, msg.more
	v.cursor = min(v.cursor, max(0, len(v.issues)-1))
}

// localIssues opens the popup for the selected local repo.
func (a *App) localIssues() tea.Cmd {
	r := a.local.selected()
	if r == nil {
		return nil
	}
	var urls []string
	if r.status != nil {
		urls = r.status.Remotes
	}
	return a.openIssues(r.rel, match.GitHubRepos(urls))
}

// remoteIssues opens the popup for the selected remote repo.
func (a *App) remoteIssues() tea.Cmd {
	r := a.remote.current()
	if r == nil {
		return nil
	}
	return a.openIssues(r.FullName(), []string{r.Owner + "/" + r.Name})
}

func (a *App) issuesKey(key string) tea.Cmd {
	v := a.issues
	page := max(1, a.issuesListHeight())
	move := func(d int) { v.cursor = max(0, min(len(v.issues)-1, v.cursor+d)) }
	switch key {
	case "esc", "q", "i":
		a.issues = nil
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "pgup":
		move(-page)
	case "pgdown":
		move(page)
	case "home":
		v.cursor = 0
	case "end":
		move(len(v.issues))
	case "r":
		return a.loadIssues()
	case "enter", "o":
		if v.cursor < len(v.issues) {
			is := v.issues[v.cursor]
			if err := launcher.OpenURL(is.URL); err != nil {
				return a.notify(2, "Opening #%d: %v", is.Number, err)
			}
			return a.notify(1, "Opened #%d in your browser", is.Number)
		}
	}
	return nil
}

// Popup geometry: the box fills the screen less a margin; inside it, a
// title, a blank line, the list, a blank line and a hint line.
func (a *App) issuesBoxSize() (w, h int) { return max(40, min(a.w-4, 120)), max(10, a.h-2) }

func (a *App) issuesListHeight() int {
	_, h := a.issuesBoxSize()
	return max(1, h-2-2-4) // border, vertical padding, title/blank/blank/hint
}

func (a *App) renderIssues() string {
	st := a.st
	v := a.issues
	bw, _ := a.issuesBoxSize()
	cw := bw - 2 - 4 // border and horizontal padding
	listH := a.issuesListHeight()

	repo := v.repo
	if repo == "" && len(v.repos) > 0 {
		repo = v.repos[0]
	}
	title := st.boxTitle.Render("Issues") + st.dim.Render(" · ") + st.bold.Render(repo)
	var count string
	switch {
	case v.loading:
		count = a.spin.View()
	case v.err == nil && v.more:
		count = st.dim.Render(fmt.Sprintf("%d+ open", len(v.issues)))
	case v.err == nil:
		count = st.dim.Render(fmt.Sprintf("%d open", len(v.issues)))
	}
	gap := max(1, cw-lipgloss.Width(title)-lipgloss.Width(count))
	lines := []string{fit(title+strings.Repeat(" ", gap)+count, cw), ""}

	var body []string
	switch {
	case v.loading && len(v.issues) == 0:
		body = []string{a.spin.View() + st.dim.Render(" Loading issues…")}
	case errors.Is(v.err, github.ErrNoToken):
		body = []string{st.warn.Render("Not signed in. ") + st.dim.Render("Run ") + st.key.Render("gh auth login") +
			st.dim.Render(" or set ") + st.key.Render("GITHUB_TOKEN") + st.dim.Render(".")}
	case errors.Is(v.err, github.ErrNotFound):
		body = []string{st.dim.Render("No issues to show: issues are turned off for this repo, or your token can't see it.")}
	case v.err != nil:
		body = []string{st.bad.Render("Couldn't load issues: ") + st.dim.Render(v.err.Error())}
	case len(v.issues) == 0:
		body = []string{st.ok.Render("✓ ") + st.dim.Render("No open issues.")}
	default:
		body = a.issueRows(cw, listH)
	}
	for len(body) < listH {
		body = append(body, "")
	}
	lines = append(lines, body[:listH]...)
	lines = append(lines, "", fit(st.key.Render("enter")+st.dim.Render(" open in browser · ")+
		st.key.Render("r")+st.dim.Render(" refresh · ")+st.key.Render("esc")+st.dim.Render(" close"), cw))
	return st.box.Width(bw).Render(strings.Join(lines, "\n"))
}

func (a *App) issueRows(cw, h int) []string {
	st := a.st
	v := a.issues
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+h {
		v.offset = v.cursor - h + 1
	}
	v.offset = max(0, min(v.offset, len(v.issues)-h))

	const numW, agoW = 7, 8
	labelW := min(28, cw/4)
	titleW := max(10, cw-2-numW-labelW-agoW-3)
	var rows []string
	for i := v.offset; i < min(len(v.issues), v.offset+h); i++ {
		is := v.issues[i]
		marker, title := "  ", st.textS.Render(is.Title)
		if i == v.cursor {
			marker, title = st.marker.Render("▌")+" ", st.selName.Render(is.Title)
		}
		labels := st.dim.Render(strings.Join(is.Labels, " · "))
		rows = append(rows, marker+fit(st.warn.Render(fmt.Sprintf("#%d", is.Number)), numW)+
			fit(title, titleW)+" "+fit(labels, labelW)+" "+
			fit(st.agoStyle(is.UpdatedAt).Render(ago(is.UpdatedAt)), agoW))
	}
	return rows
}

// Package ui is folgit's Bubble Tea interface.
package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bferg314/folgit/internal/cache"
	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/launcher"
	"github.com/bferg314/folgit/internal/match"
	"github.com/bferg314/folgit/internal/provider"
	"github.com/bferg314/folgit/internal/repos"
	"github.com/bferg314/folgit/internal/scan"
)

const (
	tabLocal = iota
	tabRemote
	tabSettings
)

var tabNames = []string{"Local", "Remote", "Settings"}

// chromeLines is the screen height used by the header, its rule and the
// status bar around the body.
const chromeLines = 3

// Keys folgit handles itself; tools bound to these are shadowed.
var reservedKeys = map[string]bool{
	"q": true, "?": true, "/": true, "r": true, "R": true, "p": true, "s": true, "g": true,
	"P": true, "F": true, "i": true, "d": true, "J": true, "K": true, "b": true, "B": true, "w": true,
	"j": true, "k": true, "1": true, "2": true, "3": true, "*": true, "h": true, "H": true,
}

// App is the root model.
type App struct {
	cfg     *config.Config
	cfgPath string
	root    string
	st      styles

	w, h     int
	tab      int
	local    localTab
	remote   remoteTab
	settings settingsTab

	spin     spinner.Model
	spinning bool
	popup    *toolPopup
	help     bool
	toast    toast

	gen    int
	cancel context.CancelFunc

	// cwdFile is set when launched through the shell wrapper (folgit init);
	// cdTarget is the repo to cd into after exit.
	cwdFile  string
	cdTarget string
	remoteAt time.Time

	startupNotice string

	bulk    *bulkOp
	issues  *issuesView
	cleanup *cleanupView
	detail  detailState
}

type toast struct {
	id   int
	text string
	kind int // 0 info, 1 ok, 2 error
}

// Messages.
type (
	eventsMsg struct {
		gen    int
		events []repos.Event
		done   bool
	}
	statusMsg struct {
		path   string
		status gitinfo.Status
	}
	remoteMsg struct {
		repos []provider.Repo
		err   error
	}
	cloneMsg struct {
		repo provider.Repo
		dest string
		err  error
	}
	pullMsg struct {
		path string
		err  error
	}
	toolExitMsg struct {
		path, name string
		err        error
	}
	toastExpireMsg struct{ id int }
)

// New builds the app for root. Rows from cached are shown immediately and
// replaced as the fresh scan reports in. cwdFile enables the "g" (cd) key.
func New(cfg *config.Config, cfgPath, root string, cached *cache.File, cwdFile string) *App {
	a := &App{
		cfg:     cfg,
		cfgPath: cfgPath,
		root:    root,
		st:      newStyles(true),
		local:   newLocalTab(cfg),
		remote:  newRemoteTab(),
		detail:  detailState{show: true, cache: make(map[string]*gitinfo.Details)},
		cwdFile: cwdFile,
	}
	a.spin = spinner.New(spinner.WithSpinner(spinner.MiniDot))
	a.spin.Style = a.st.logo

	if cached != nil {
		for _, l := range cached.Local {
			a.local.add(l.Path, root, 0).status = l.Status
		}
		a.remote.all = cached.Remote
		a.remoteAt = cached.RemoteAt
		a.refreshViews()
	}
	return a
}

// Snapshot captures what should be cached for the next start.
func (a *App) Snapshot() *cache.File {
	f := &cache.File{Root: a.root, Remote: a.remote.all, RemoteAt: a.remoteAt}
	for _, r := range a.local.rows {
		l := cache.Local{Path: r.path}
		if r.status != nil && r.status.Err == nil {
			l.Status = r.status
		}
		f.Local = append(f.Local, l)
	}
	return f
}

// CdTarget is the repo the user asked to cd into, or "".
func (a *App) CdTarget() string { return a.cdTarget }

// saveCache writes a snapshot in the background. Failures are ignored: the
// cache is only a startup accelerator.
func (a *App) saveCache() tea.Cmd {
	snap := a.Snapshot()
	return func() tea.Msg {
		_ = cache.Save(snap)
		return nil
	}
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor, a.startScan(), a.loadRemote()}
	if a.startupNotice != "" {
		cmds = append(cmds, a.notify(2, "%s", a.startupNotice))
	}
	return tea.Batch(cmds...)
}

// StartupNotice shows msg as a message once the UI starts.
func (a *App) StartupNotice(msg string) { a.startupNotice = msg }

// startScan (re)scans the root. Existing rows keep their last status until
// fresh results arrive; rows not rediscovered are dropped when it finishes.
func (a *App) startScan() tea.Cmd {
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.gen++
	a.local.scanning = true
	a.local.events = repos.Load(ctx, a.root, scan.Options{
		MaxDepth: a.cfg.MaxDepth,
		Hidden:   a.cfg.ScanHidden,
		Ignore:   a.cfg.Ignore,
	})
	return tea.Batch(a.continueScan(), a.startSpinner())
}

// waitEvents blocks for the next event, then drains whatever else is ready so
// a burst of results causes one re-render instead of hundreds.
func waitEvents(gen int, ch <-chan repos.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return eventsMsg{gen: gen, done: true}
		}
		evs := []repos.Event{e}
		for len(evs) < 512 {
			select {
			case e, ok := <-ch:
				if !ok {
					return eventsMsg{gen: gen, events: evs, done: true}
				}
				evs = append(evs, e)
			default:
				return eventsMsg{gen: gen, events: evs}
			}
		}
		return eventsMsg{gen: gen, events: evs}
	}
}

func (a *App) loadRemote() tea.Cmd {
	a.remote.loading = true
	a.remote.err = nil
	opt := a.cfg.GitHub
	return tea.Batch(a.startSpinner(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		gh, err := githubClient(ctx, opt)
		if err != nil {
			return remoteMsg{err: err}
		}
		rs, err := gh.ListRepos(ctx)
		return remoteMsg{repos: rs, err: err}
	})
}

func refreshStatus(path string) tea.Cmd {
	return func() tea.Msg {
		return statusMsg{path: path, status: gitinfo.Get(context.Background(), path)}
	}
}

func (a *App) busy() bool {
	return a.local.scanning || a.remote.loading || a.local.anyBusy() || len(a.remote.cloning) > 0 ||
		a.bulk != nil || a.detailLoading() || a.issues != nil && a.issues.loading ||
		a.cleanup != nil && (a.cleanup.loading || a.cleanup.deleting)
}

func (a *App) startSpinner() tea.Cmd {
	if a.spinning {
		return nil
	}
	a.spinning = true
	return a.spin.Tick
}

func (a *App) notify(kind int, format string, args ...any) tea.Cmd {
	a.toast.id++
	a.toast.text = fmt.Sprintf(format, args...)
	a.toast.kind = kind
	id := a.toast.id
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return toastExpireMsg{id} })
}

// localKeys is the set of match keys for every remote of every local repo.
func (a *App) localKeys() map[string]bool {
	keys := make(map[string]bool)
	for _, r := range a.local.rows {
		if r.status == nil {
			continue
		}
		for _, u := range r.status.Remotes {
			if k := match.Key(u); k != "" {
				keys[k] = true
			}
		}
	}
	return keys
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := a.dispatch(msg)
	// Any message can move the selection (keys, filtering, rows streaming
	// in), so keep the detail pane in step afterwards.
	return a, tea.Batch(cmd, a.syncDetails())
}

func (a *App) dispatch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		return a, nil

	case tea.BackgroundColorMsg:
		a.st = newStyles(msg.IsDark())
		a.spin.Style = a.st.logo
		return a, nil

	case spinner.TickMsg:
		if !a.busy() {
			a.spinning = false
			return a, nil
		}
		var cmd tea.Cmd
		a.spin, cmd = a.spin.Update(msg)
		return a, cmd

	case eventsMsg:
		if msg.gen != a.gen {
			return a, nil
		}
		a.local.apply(msg.events, a.root, a.gen)
		for _, e := range msg.events {
			if e.Status != nil {
				a.invalidateDetails(e.Path)
			}
		}
		var cmd tea.Cmd
		if msg.done {
			a.local.scanning = false
			a.local.prune(a.gen)
			cmd = a.saveCache()
		}
		a.local.refresh()
		a.remote.refresh(a.localKeys())
		if !msg.done {
			cmd = a.continueScan()
		}
		return a, cmd

	case statusMsg:
		if r := a.local.rows[msg.path]; r != nil {
			s := msg.status
			r.status = &s
			r.busy = ""
		}
		a.invalidateDetails(msg.path)
		a.local.refresh()
		a.remote.refresh(a.localKeys())
		return a, nil

	case bulkItemMsg:
		return a, a.handleBulkItem(msg)

	case detailTickMsg:
		if msg.seq != a.detail.seq || a.detail.cache[msg.path] != nil {
			return a, nil
		}
		return a, tea.Batch(loadDetails(msg.path), a.startSpinner())

	case cleanupScanMsg:
		a.handleCleanupScan(msg)
		return a, nil

	case cleanupDoneMsg:
		return a, a.handleCleanupDone(msg)

	case issuesMsg:
		a.handleIssues(msg)
		return a, nil

	case issueCreatedMsg:
		return a, a.handleIssueCreated(msg)

	case detailMsg:
		d := msg.details
		a.detail.cache[msg.path] = &d
		return a, nil

	case remoteMsg:
		a.remote.loading = false
		a.remote.err = msg.err
		if msg.err != nil {
			return a, nil
		}
		a.remote.all = msg.repos
		a.remoteAt = time.Now()
		a.remote.refresh(a.localKeys())
		return a, a.saveCache()

	case cloneMsg:
		key := msg.repo.Key()
		delete(a.remote.cloning, key)
		if msg.err != nil {
			a.remote.failed[key] = msg.err.Error()
			return a, a.notify(2, "Clone %s failed: %v", msg.repo.FullName(), msg.err)
		}
		a.remote.cloned[key] = true
		a.remote.refresh(a.localKeys())
		a.local.add(msg.dest, a.root, a.gen)
		a.local.refresh()
		return a, tea.Batch(refreshStatus(msg.dest), a.notify(1, "Cloned %s", msg.repo.FullName()))

	case pullMsg:
		cmds := []tea.Cmd{refreshStatus(msg.path)}
		name := filepath.Base(msg.path)
		if msg.err != nil {
			if r := a.local.rows[msg.path]; r != nil {
				r.busy = ""
			}
			cmds = append(cmds, a.notify(2, "Pull %s failed: %v", name, msg.err))
		} else {
			cmds = append(cmds, a.notify(1, "Pulled %s", name))
		}
		return a, tea.Batch(cmds...)

	case toolExitMsg:
		cmds := []tea.Cmd{refreshStatus(msg.path)}
		if msg.err != nil {
			cmds = append(cmds, a.notify(2, "%s exited: %v", msg.name, msg.err))
		}
		return a, tea.Batch(cmds...)

	case toastExpireMsg:
		if msg.id == a.toast.id {
			a.toast.text = ""
		}
		return a, nil

	case tea.KeyPressMsg:
		return a, a.handleKey(msg)
	}

	// Let a focused input receive anything else (pastes, cursor blink etc).
	if a.issues != nil {
		return a, a.updateIssueForm(msg)
	}
	return a, a.updateFilter(msg)
}

// continueScan re-arms the event reader for the current generation.
func (a *App) continueScan() tea.Cmd {
	return waitEvents(a.gen, a.local.events)
}

func (a *App) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+c" {
		return tea.Quit
	}
	if a.help {
		a.help = false
		return nil
	}
	if a.cleanup != nil {
		return a.cleanupKey(key)
	}
	if a.issues != nil {
		return a.issuesKey(msg)
	}
	if a.popup != nil {
		return a.popupKey(key)
	}
	if f := a.activeFilter(); f != nil && f.Focused() {
		switch key {
		case "esc":
			f.SetValue("")
			f.Blur()
		case "enter", "up", "down":
			f.Blur()
		default:
			cmd := a.updateFilter(msg)
			a.refreshViews()
			return cmd
		}
		a.refreshViews()
		return nil
	}

	switch key {
	case "q":
		return tea.Quit
	case "?":
		a.help = true
		return nil
	case "tab", "right":
		return a.setTab((a.tab + 1) % len(tabNames))
	case "shift+tab", "left":
		return a.setTab((a.tab + len(tabNames) - 1) % len(tabNames))
	case "1", "2", "3":
		return a.setTab(int(key[0] - '1'))
	case "/":
		if f := a.activeFilter(); f != nil {
			return f.Focus()
		}
	case "esc":
		if f := a.activeFilter(); f != nil && f.Value() != "" {
			f.SetValue("")
			a.refreshViews()
			return nil
		}
		if a.detailLayout() == layoutFull {
			a.detail.full = false
			return nil
		}
	}

	switch a.tab {
	case tabLocal:
		return a.localKey(key)
	case tabRemote:
		return a.remoteKey(key)
	case tabSettings:
		return a.settingsKey(key)
	}
	return nil
}

// activeFilter is the filter input of the current tab, if it has one.
func (a *App) activeFilter() *textinput.Model {
	switch a.tab {
	case tabLocal:
		return &a.local.filter
	case tabRemote:
		return &a.remote.filter
	}
	return nil
}

func (a *App) refreshViews() {
	a.local.refresh()
	a.remote.refresh(a.localKeys())
}

func (a *App) updateFilter(msg tea.Msg) tea.Cmd {
	f := a.activeFilter()
	if f == nil || !f.Focused() {
		return nil
	}
	var cmd tea.Cmd
	*f, cmd = f.Update(msg)
	return cmd
}

// launch runs tool t in path.
func (a *App) launch(t config.Tool, path string) tea.Cmd {
	if !config.Available(t.Cmd) {
		return a.notify(2, "%s: %q not found on PATH", t.Name, t.Cmd)
	}
	cmd := launcher.Command(t, path)
	if t.Mode == config.ModeTerminal {
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			return toolExitMsg{path: path, name: t.Name, err: err}
		})
	}
	if err := launcher.Detach(cmd); err != nil {
		return a.notify(2, "%s: %v", t.Name, err)
	}
	return a.notify(1, "Opened %s in %s", filepath.Base(path), t.Name)
}

// enabledTools returns the tools switched on in settings.
func (a *App) enabledTools() []config.Tool {
	var ts []config.Tool
	for _, t := range a.cfg.Tools {
		if t.Enabled {
			ts = append(ts, t)
		}
	}
	return ts
}

func (a *App) saveConfig() tea.Cmd {
	if err := a.cfg.Save(a.cfgPath); err != nil {
		return a.notify(2, "Saving config: %v", err)
	}
	return nil
}

// ---- View ----

func (a *App) View() tea.View {
	v := tea.NewView(a.render())
	v.AltScreen = true
	v.WindowTitle = "folgit"
	return v
}

func (a *App) render() string {
	if a.w == 0 || a.h == 0 {
		return ""
	}
	header := a.renderHeader()
	rule := a.st.rule.Render(strings.Repeat("─", a.w))

	bodyH := a.h - chromeLines
	var body string
	switch a.tab {
	case tabLocal:
		body = a.renderLocalBody(bodyH)
	case tabRemote:
		body = a.renderRemote(bodyH)
	case tabSettings:
		body = a.renderSettings(bodyH)
	}
	body = padLines(body, bodyH, a.w)

	base := strings.Join([]string{header, rule, body, a.renderStatusBar()}, "\n")

	var overlay string
	switch {
	case a.help:
		overlay = a.renderHelp()
	case a.cleanup != nil:
		overlay = a.renderCleanup()
	case a.issues != nil:
		overlay = a.renderIssues()
	case a.popup != nil:
		overlay = a.renderPopup()
	}
	if overlay == "" {
		return base
	}
	ow, oh := lipgloss.Width(overlay), lipgloss.Height(overlay)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(overlay).X(max(0, (a.w-ow)/2)).Y(max(0, (a.h-oh)/2)).Z(1),
	).Render()
}

func padLines(s string, h, w int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = fit(l, w)
	}
	return strings.Join(lines, "\n")
}

func (a *App) renderHeader() string {
	logo := a.st.logo.Render(" ◆ folgit ")
	counts := []string{
		fmt.Sprintf("%d", len(a.local.rows)-a.local.hiddenCount()),
		fmt.Sprintf("%d", a.remote.missing),
		"",
	}
	var tabs []string
	for i, name := range tabNames {
		label := name
		if counts[i] != "" {
			label += " " + counts[i]
		}
		if i == a.tab {
			tabs = append(tabs, a.st.tabOn.Render(label))
		} else {
			tabs = append(tabs, a.st.tabOff.Render(label))
		}
	}
	left := logo + " " + strings.Join(tabs, " ")

	right := a.st.dim.Render(tildePath(a.root)) + " "
	gap := a.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fit(left, a.w)
	}
	return left + strings.Repeat(" ", gap) + right
}

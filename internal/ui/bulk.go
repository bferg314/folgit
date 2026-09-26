package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/launcher"
)

// At most this many repos fetch at once.
var bulkSlots = make(chan struct{}, 8)

type outcome int

const (
	outFetched outcome = iota // fetch only
	outUpdated                // fast-forwarded
	outCurrent                // nothing to pull
	outSkipped                // dirty, diverged or no upstream
	outFailed
)

// bulkOp tracks a fetch-all or pull-all in progress.
type bulkOp struct {
	pull     bool
	total    int
	done     int
	counts   [5]int
	behind   int
	firstErr string
}

type bulkItemMsg struct {
	path    string
	status  gitinfo.Status
	outcome outcome
	err     error
}

// startBulk fetches every repo that has a remote. With pull, it then
// fast-forwards repos that are clean, behind and not ahead; everything else
// is left alone.
func (a *App) startBulk(pull bool) tea.Cmd {
	if a.bulk != nil {
		return a.notify(2, "A bulk operation is already running")
	}
	var paths []string
	for _, r := range a.local.rows {
		if r.busy == "" && r.status != nil && r.status.Err == nil && len(r.status.Remotes) > 0 {
			paths = append(paths, r.path)
		}
	}
	if len(paths) == 0 {
		return a.notify(2, "No repositories with remotes to fetch")
	}

	a.bulk = &bulkOp{pull: pull, total: len(paths)}
	label := "fetching"
	if pull {
		label = "pulling"
	}
	cmds := []tea.Cmd{a.startSpinner()}
	for _, p := range paths {
		a.local.rows[p].busy = label
		cmds = append(cmds, bulkItem(p, pull))
	}
	return tea.Batch(cmds...)
}

func bulkItem(path string, pull bool) tea.Cmd {
	return func() tea.Msg {
		bulkSlots <- struct{}{}
		defer func() { <-bulkSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		msg := bulkItemMsg{path: path, outcome: outFetched}
		if err := launcher.Fetch(ctx, path); err != nil {
			msg.outcome, msg.err = outFailed, err
		} else if pull {
			s := gitinfo.Get(ctx, path)
			switch {
			case s.Upstream == "" || s.Dirty() || s.Ahead > 0 && s.Behind > 0:
				msg.outcome = outSkipped
			case s.Behind == 0:
				msg.outcome = outCurrent
			default:
				if err := launcher.Pull(ctx, path); err != nil {
					msg.outcome, msg.err = outFailed, err
				} else {
					msg.outcome = outUpdated
				}
			}
		}
		msg.status = gitinfo.Get(context.Background(), path)
		return msg
	}
}

func (a *App) handleBulkItem(msg bulkItemMsg) tea.Cmd {
	if r := a.local.rows[msg.path]; r != nil {
		s := msg.status
		r.status = &s
		r.busy = ""
	}
	a.invalidateDetails(msg.path)
	a.refreshViews()

	b := a.bulk
	if b == nil {
		return nil
	}
	b.done++
	b.counts[msg.outcome]++
	if msg.status.Behind > 0 {
		b.behind++
	}
	if msg.err != nil && b.firstErr == "" {
		b.firstErr = filepath.Base(msg.path) + ": " + msg.err.Error()
	}
	if b.done < b.total {
		return nil
	}

	a.bulk = nil
	var parts []string
	if b.pull {
		for _, p := range []struct {
			n    int
			word string
		}{{b.counts[outUpdated], "updated"}, {b.counts[outCurrent], "up to date"}, {b.counts[outSkipped], "skipped"}} {
			if p.n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", p.n, p.word))
			}
		}
	} else {
		parts = append(parts, fmt.Sprintf("Fetched %d repos", b.total-b.counts[outFailed]))
		if b.behind > 0 {
			parts = append(parts, fmt.Sprintf("%d behind (P pulls them)", b.behind))
		}
	}
	kind := 1
	if n := b.counts[outFailed]; n > 0 {
		kind = 2
		parts = append(parts, fmt.Sprintf("%d failed (%s)", n, b.firstErr))
	}
	return tea.Batch(a.notify(kind, "%s", strings.Join(parts, " · ")), a.saveCache())
}

// bulkProgress is shown in the header while a bulk operation runs.
func (a *App) bulkProgress() string {
	if a.bulk == nil {
		return ""
	}
	verb := "fetching"
	if a.bulk.pull {
		verb = "pulling"
	}
	return fmt.Sprintf("%s %d/%d", verb, a.bulk.done, a.bulk.total)
}

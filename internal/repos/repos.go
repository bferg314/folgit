// Package repos combines the directory scan with per-repo status workers into
// a single stream of events.
package repos

import (
	"context"
	"runtime"
	"sync"

	"github.com/bferg314/folgit/internal/gitinfo"
	"github.com/bferg314/folgit/internal/scan"
)

// Event reports a discovered repository (Status == nil) or its loaded status.
type Event struct {
	Path   string
	Status *gitinfo.Status
}

// Load scans root and streams events. The channel is closed when every
// discovered repository has had its status loaded.
func Load(ctx context.Context, root string, opt scan.Options) <-chan Event {
	out := make(chan Event, 256)
	paths := make(chan string, 256)

	send := func(e Event) {
		select {
		case out <- e:
		case <-ctx.Done():
		}
	}

	go func() {
		defer close(paths)
		scan.Walk(ctx, root, opt, func(p string) {
			send(Event{Path: p})
			select {
			case paths <- p:
			case <-ctx.Done():
			}
		})
	}()

	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range paths {
				s := gitinfo.Get(ctx, p)
				send(Event{Path: p, Status: &s})
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

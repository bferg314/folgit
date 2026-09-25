// Package scan finds git repositories below a directory using a parallel walk.
package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Options controls the walk.
type Options struct {
	MaxDepth int
	Hidden   bool     // descend into dot-directories
	Ignore   []string // directory names to skip
}

// Walk calls found for every repository below root. found may be called from
// multiple goroutines. Walk returns once the whole tree has been visited.
//
// Walking stops descending once a repository is found (except at the root),
// so nested clones inside a repo are not listed. Symlinked directories are
// skipped to avoid cycles.
func Walk(ctx context.Context, root string, opt Options, found func(path string)) {
	w := &walker{
		opt:    opt,
		found:  found,
		sem:    make(chan struct{}, runtime.NumCPU()*4),
		ignore: make(map[string]bool, len(opt.Ignore)),
	}
	for _, name := range opt.Ignore {
		w.ignore[name] = true
	}
	w.visit(ctx, filepath.Clean(root), 0)
	w.wg.Wait()
}

type walker struct {
	opt    Options
	found  func(string)
	sem    chan struct{}
	wg     sync.WaitGroup
	ignore map[string]bool
}

func (w *walker) visit(ctx context.Context, dir string, depth int) {
	if ctx.Err() != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		// .git is a directory for normal clones and a file for worktrees
		// and submodules; both count.
		if e.Name() == ".git" {
			w.found(dir)
			if depth > 0 {
				return
			}
			break
		}
	}
	if depth >= w.opt.MaxDepth {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == ".git" || w.ignore[name] || (!w.opt.Hidden && strings.HasPrefix(name, ".")) {
			continue
		}
		child := filepath.Join(dir, name)
		select {
		case w.sem <- struct{}{}:
			w.wg.Add(1)
			go func() {
				defer w.wg.Done()
				defer func() { <-w.sem }()
				w.visit(ctx, child, depth+1)
			}()
		default:
			// All workers busy: walk inline rather than block.
			w.visit(ctx, child, depth+1)
		}
	}
}

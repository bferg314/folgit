// Command folgit is a terminal dashboard for every git repository under a
// directory, plus the GitHub repositories you haven't cloned yet.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/cache"
	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/shell"
	"github.com/bferg314/folgit/internal/ui"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	cwdFile := flag.String("cwd-file", "", "on exit via \"g\", write the selected repo path here (used by the shell wrapper)")
	noCache := flag.Bool("no-cache", false, "ignore the cached scan and start fresh")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage:
  folgit [flags] [dir]    scan dir (default: current directory)
  folgit init <shell>     print shell integration (%s)

Flags:
`, strings.Join(shell.Names(), ", "))
		flag.PrintDefaults()
	}
	flag.Parse()

	var err error
	switch {
	case *showVersion:
		fmt.Println("folgit", version)
	case flag.Arg(0) == "init":
		err = runInit(flag.Arg(1))
	default:
		err = run(flag.Arg(0), *cwdFile, *noCache)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "folgit:", err)
		os.Exit(1)
	}
}

func runInit(sh string) error {
	if sh == "" {
		fmt.Fprintln(os.Stderr, "Add one of these to your shell's startup file:")
		for _, n := range []string{"bash", "zsh", "fish", "powershell"} {
			fmt.Fprintln(os.Stderr, "  "+shell.Install(n))
		}
		return fmt.Errorf("missing shell name")
	}
	s, err := shell.Script(sh)
	if err != nil {
		return err
	}
	fmt.Print(s)
	return nil
}

func run(dir, cwdFile string, noCache bool) error {
	if dir == "" {
		dir = "."
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	cfgPath, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	var cached *cache.File
	if !noCache {
		cached = cache.Load(root)
	}

	app := ui.New(cfg, cfgPath, root, cached, cwdFile)
	if _, err := tea.NewProgram(app).Run(); err != nil {
		return err
	}

	// Capture anything that changed since the last background save
	// (pulls, clones, tool sessions).
	_ = cache.Save(app.Snapshot())

	if cwdFile != "" && app.CdTarget() != "" {
		return os.WriteFile(cwdFile, []byte(app.CdTarget()), 0o644)
	}
	return nil
}

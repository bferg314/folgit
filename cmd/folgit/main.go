// Command folgit is a terminal dashboard for every git repository under a
// directory, plus the GitHub repositories you haven't cloned yet.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/ui"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: folgit [dir]\n\nScans dir (default: current directory) for git repositories.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("folgit", version)
		return
	}

	if err := run(flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "folgit:", err)
		os.Exit(1)
	}
}

func run(dir string) error {
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

	_, err = tea.NewProgram(ui.New(cfg, cfgPath, root)).Run()
	return err
}

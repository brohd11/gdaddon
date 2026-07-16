// Command repoview shows git status across every repository nested under a directory — branch,
// uncommitted changes, ahead/behind — on a fresh scan each run, and drives fetch/pull/push/commit
// through a shared TUI. It's the manifest-free sibling of gdaddon, built on the same bubblestack
// framework and gitstack git engine/screens.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/brohd11/repoview/internal/app"
)

func main() {
	depth := flag.Int("depth", 5, "maximum directory depth to scan for git repos")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, "repoview — git status across every repo under a directory\n\n"+
			"usage: repoview [flags] [dir]   (dir defaults to the current directory)\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := app.Run(abs, *depth); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

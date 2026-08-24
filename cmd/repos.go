package cmd

import (
	"github.com/brohd11/gitstack/repocmd"
)

// The command itself lives in gitstack/repocmd, beside the engine it drives — repoview
// ships the same one. gdaddon's copy used to be a stale fork of it: it printed straight
// to os.Stdout and returned bare errors, never having picked up the fixes repoview made.
// What stays gdaddon's is the deeper default, since a project tree nests further than a
// repo checkout does.
func init() {
	rootCmd.AddCommand(repocmd.New(repocmd.Options{
		AppName:      "gdaddon",
		DefaultDepth: 5,
	}))
}

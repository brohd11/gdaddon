package cmd

import (
	"github.com/brohd11/gitstack/repocmd"
)

// The command lives in gitstack/repocmd; gdaddon only sets a deeper default depth, since
// project trees nest further than repo checkouts.
func init() {
	rootCmd.AddCommand(repocmd.New(repocmd.Options{
		AppName:      "gdaddon",
		DefaultDepth: 5,
	}))
}

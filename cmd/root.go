package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "gdutil",
	Short:         "Godot development utilities",
	SilenceUsage:  true, // don't dump usage on runtime (non-flag) errors
	SilenceErrors: false,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

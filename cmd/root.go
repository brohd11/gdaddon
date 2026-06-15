package cmd

import (
	"fmt"
	"os"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui"

	"github.com/spf13/cobra"
)

var installFlag bool

var rootCmd = &cobra.Command{
	Use:           "gdaddon [yaml_path] [project_root]",
	Short:         "Browse and install Godot addons (interactive TUI by default; --install for non-interactive)",
	Args:          cobra.RangeArgs(0, 2),
	SilenceUsage:  true, // don't dump usage on runtime (non-flag) errors
	SilenceErrors: false,
	RunE:          runRoot,
}

func init() {
	rootCmd.Flags().BoolVar(&installFlag, "install", false, "install addons from the manifest non-interactively, then exit")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// runRoot resolves the manifest/project paths and either runs the non-interactive
// install (--install) or launches the TUI (default).
func runRoot(cmd *cobra.Command, args []string) error {
	yamlFile, projectRoot, err := resolvePaths(args)
	if err != nil {
		return err
	}
	if installFlag {
		return runInstall(yamlFile, projectRoot)
	}
	return tui.Run(yamlFile, projectRoot)
}

// runInstall is the non-interactive path: inspect the manifest and install/update
// everything, reporting progress to stdout.
func runInstall(yamlFile, projectRoot string) error {
	statuses, err := addon.Inspect(yamlFile, projectRoot)
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		fmt.Println("No addons found in YAML.")
		return nil
	}
	report := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	return addon.InstallAll(yamlFile, statuses, projectRoot, report)
}

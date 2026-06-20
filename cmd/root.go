package cmd

import (
	"context"
	"fmt"
	"os"

	"gdaddon/internal/addon"
	"gdaddon/internal/config"
	"gdaddon/internal/tui"

	"github.com/spf13/cobra"
)

var installFlag bool

var rootCmd = &cobra.Command{
	Use:           "gdaddon [project_root]",
	Short:         "Browse and install Godot addons (interactive TUI by default; --install for non-interactive)",
	Args:          cobra.MaximumNArgs(1),
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

// runRoot resolves the project root and either runs the non-interactive install
// (--install) or launches the TUI (default). The manifest is discovered under the root
// (by runInstall here, or the TUI context's scan).
func runRoot(cmd *cobra.Command, args []string) error {
	// Dump the default config.yml on first run so it's the editable source of
	// truth (search sources, archive dir). A failure here is non-fatal.
	if created, path, err := config.Ensure(); err == nil && created {
		fmt.Fprintf(os.Stderr, "wrote default config to %s\n", path)
	}

	projectRoot, err := resolveRoot(args)
	if err != nil {
		return err
	}
	if installFlag {
		return runInstall(projectRoot)
	}
	return tui.Run(projectRoot)
}

// runInstall is the non-interactive path: discover the manifest under the project root,
// inspect it, and install/update everything, reporting progress to stdout.
func runInstall(projectRoot string) error {
	manifest, err := addon.FindManifest(projectRoot)
	if err != nil {
		return err
	}
	if manifest == "" {
		return fmt.Errorf("no addon_manifest.yml found under %s; create one in the TUI or add it manually", projectRoot)
	}
	statuses, err := addon.Inspect(manifest, projectRoot)
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		fmt.Println("No addons found in YAML.")
		return nil
	}
	report := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	return addon.InstallAll(context.Background(), manifest, statuses, projectRoot, report)
}

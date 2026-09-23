package cmd

import (
	"fmt"
	"os"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/config"
	"github.com/brohd11/gdaddon/internal/tui"

	"github.com/spf13/cobra"
)

// version is stamped by the makefile via -X ldflags; "dev" for a plain go build.
var version = "dev"

// firstRun records whether ~/.gdaddon was absent at startup (before config.Ensure), so the
// TUI can show the welcome popup.
var firstRun bool

var rootCmd = &cobra.Command{
	Use:               "gdaddon [project_root]",
	Short:             "Browse and install Godot addons (interactive TUI by default)",
	Version:           version,
	Args:              cobra.MaximumNArgs(1),
	SilenceUsage:      true, // don't dump usage on runtime (non-flag) errors
	SilenceErrors:     false,
	PersistentPreRunE: bootstrap,
	RunE:              runRoot,
}

func init() {
	rootCmd.SetVersionTemplate("gdaddon {{.Version}}\n")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// bootstrap prepares ~/.gdaddon before any command, so non-interactive subcommands get
// the same config as the TUI. It prints to stderr, keeping `list --json` output pure.
func bootstrap(cmd *cobra.Command, args []string) error {
	// Sampled before Ensure: a missing ~/.gdaddon means a first run (a deleted config file
	// does not).
	firstRun = isFirstRun()

	// Write the default config files on first run so they can be edited; failures are
	// non-fatal.
	if created, err := config.Ensure(); err == nil {
		for _, path := range created {
			fmt.Fprintf(os.Stderr, "wrote default config to %s\n", path)
		}
	}
	// Ship a .gitignore so the OS binary (~/.gdaddon/bin) isn't committed if the
	// user version-controls ~/.gdaddon. Non-fatal; existing files are left alone.
	if created, path, err := config.EnsureGitignore(); err == nil && created {
		fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	}
	return nil
}

// runRoot resolves the project root and launches the TUI. Every non-interactive mode is
// a subcommand (install / list / update-addons / repos), so this path does one thing.
func runRoot(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveRoot(args)
	if err != nil || projectRoot == "" {
		return err
	}
	return tui.Run(projectRoot, version, firstRun)
}

// isFirstRun reports whether ~/.gdaddon is absent. Call it before config.Ensure.
func isFirstRun() bool {
	dir, err := config.Dir()
	if err != nil {
		return false
	}
	_, err = os.Stat(dir)
	return os.IsNotExist(err)
}

// stdoutReport is the addon.Reporter for non-interactive subcommands: the progress lines
// the TUI logs, one per stdout line.
func stdoutReport(format string, a ...any) { fmt.Printf(format+"\n", a...) }

// inspectManifest finds the manifest under projectRoot and inspects every entry.
func inspectManifest(projectRoot string) (string, []addon.Status, error) {
	manifest, err := discoverManifest(projectRoot)
	if err != nil {
		return "", nil, err
	}
	statuses, err := addon.Inspect(manifest, projectRoot)
	return manifest, statuses, err
}

// discoverManifest finds the manifest under the project root, or errors when there is none.
func discoverManifest(projectRoot string) (string, error) {
	manifest, err := addon.FindManifest(projectRoot)
	if err != nil {
		return "", err
	}
	if manifest == "" {
		return "", fmt.Errorf("no addon_manifest.yml found under %s; create one in the TUI or add it manually", projectRoot)
	}
	return manifest, nil
}

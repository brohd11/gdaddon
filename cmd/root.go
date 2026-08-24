package cmd

import (
	"fmt"
	"os"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/config"
	"github.com/brohd11/gdaddon/internal/tui"

	"github.com/spf13/cobra"
)

// version is the binary version, injected at build time via ldflags
// (-X gdaddon/cmd.version=...); defaults to "dev" for a plain `go build`.
var version = "dev"

// firstRun records whether ~/.gdaddon was absent when this process started, sampled in
// bootstrap() before config.Ensure creates it. runRoot reads it to decide whether the
// TUI opens the welcome popup.
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

// bootstrap prepares ~/.gdaddon before any command runs. It is persistent (and no
// subcommand overrides it) so a non-interactive run gets the same config the TUI does —
// the modes used to be root flags handled inside runRoot, and moving them to
// subcommands would otherwise have skipped this.
//
// Everything it prints goes to stderr, keeping `gdaddon list --json`'s stdout pure JSON.
func bootstrap(cmd *cobra.Command, args []string) error {
	// Sampled before Ensure creates it: no ~/.gdaddon means the user has never run
	// gdaddon, which is when the TUI offers the docs. (Ensure's created-paths return
	// would also fire for someone who merely deleted one config file.)
	firstRun = isFirstRun()

	// Dump the default config files on first run so they're the editable source
	// of truth (config.yml: archive dir/theme; sources.yml: search/vcs rules). A
	// failure here is non-fatal.
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
	if err != nil {
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

// discoverManifest finds the manifest under the project root, returning a helpful
// error when there isn't one. Shared by the read-only and whole-manifest paths, which
// have nothing to do without one — unlike a targeted `install <owner/repo>`, which
// bootstraps a manifest instead (findOrCreateManifest in addoninstall.go).
// stdoutReport is the addon.Reporter the non-interactive subcommands pass into the install
// and update flows: the same progress lines the TUI streams into its log pane, printed to
// stdout one per line. The flows format their own messages, so this only adds the newline.
func stdoutReport(format string, a ...any) { fmt.Printf(format+"\n", a...) }

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

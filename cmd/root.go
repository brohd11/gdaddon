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

// version is the binary version, injected at build time via ldflags
// (-X gdaddon/cmd.version=...); defaults to "dev" for a plain `go build`.
var version = "dev"

var (
	installFlag bool
	listFlag    bool
	updateFlag  bool
)

var rootCmd = &cobra.Command{
	Use:           "gdaddon [project_root]",
	Short:         "Browse and install Godot addons (interactive TUI by default; --install for non-interactive)",
	Version:       version,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true, // don't dump usage on runtime (non-flag) errors
	SilenceErrors: false,
	RunE:          runRoot,
}

func init() {
	rootCmd.SetVersionTemplate("gdaddon {{.Version}}\n")
	rootCmd.Flags().BoolVar(&installFlag, "install", false, "install addons from the manifest non-interactively, then exit")
	rootCmd.Flags().BoolVar(&listFlag, "list", false, "print the manifest's install status without installing, then exit")
	rootCmd.Flags().BoolVar(&updateFlag, "update", false, "update installed addons to their latest release non-interactively, then exit")
	rootCmd.MarkFlagsMutuallyExclusive("install", "list", "update")
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
	// Ship a .gitignore so the OS binary (~/.gdaddon/bin) isn't committed if the
	// user version-controls ~/.gdaddon. Non-fatal; existing files are left alone.
	if created, path, err := config.EnsureGitignore(); err == nil && created {
		fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	}

	projectRoot, err := resolveRoot(args)
	if err != nil {
		return err
	}
	switch {
	case installFlag:
		return runInstall(projectRoot)
	case listFlag:
		return runList(projectRoot)
	case updateFlag:
		return runUpdate(projectRoot)
	}
	return tui.Run(projectRoot)
}

// discoverManifest finds the manifest under the project root, returning a helpful
// error when there isn't one (shared by the non-interactive paths).
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

// runList is the read-only path: discover the manifest, inspect it, and print each
// addon's local state and version without touching the filesystem.
func runList(projectRoot string) error {
	manifest, err := discoverManifest(projectRoot)
	if err != nil {
		return err
	}
	statuses, err := addon.Inspect(manifest, projectRoot)
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		fmt.Println("No addons found in YAML.")
		return nil
	}
	for _, s := range statuses {
		ver := s.Addon.Version
		if ver == "" {
			ver = "-"
		}
		local := s.LocalVersion
		if local == "" {
			local = "-"
		}
		fmt.Printf("%-12s %-24s local=%s pinned=%s\n", s.State.String(), s.Addon.Name, local, ver)
	}
	return nil
}

// runUpdate is the non-interactive update path: discover the manifest, resolve an
// update plan for every installed addon with a newer release, and install them,
// reporting progress to stdout.
func runUpdate(projectRoot string) error {
	manifest, err := discoverManifest(projectRoot)
	if err != nil {
		return err
	}
	plans, err := addon.ResolveUpdatePlans(context.Background(), manifest, projectRoot)
	if err != nil {
		return err
	}
	if len(plans) == 0 {
		fmt.Println("All installed addons are up to date.")
		return nil
	}
	report := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	_, err = addon.UpdateAll(context.Background(), manifest, plans, projectRoot, report)
	return err
}

// runInstall is the non-interactive path: discover the manifest under the project root,
// inspect it, and install/update everything, reporting progress to stdout.
func runInstall(projectRoot string) error {
	manifest, err := discoverManifest(projectRoot)
	if err != nil {
		return err
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
	_, err = addon.InstallAll(context.Background(), manifest, statuses, projectRoot, report)
	return err
}

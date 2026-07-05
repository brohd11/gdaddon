package cmd

import (
	"context"
	"encoding/json"
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
	installFlag      bool
	listFlag         bool
	updateFlag       bool
	jsonFlag         bool
	checkUpdatesFlag bool
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
	rootCmd.Flags().BoolVar(&jsonFlag, "json", false, "with --list, print status as JSON for machine consumption")
	rootCmd.Flags().BoolVar(&checkUpdatesFlag, "check-updates", false, "with --list --json, also check each addon for a newer release (network)")
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
	return tui.Run(projectRoot, version)
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
	if jsonFlag {
		return printListJSON(statuses, projectRoot)
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

// listEntryJSON is the stable, machine-parseable shape of one addon's status,
// emitted by `--list --json` for the GDScript side to consume.
type listEntryJSON struct {
	Name          string        `json:"name"`
	State         string        `json:"state"` // missing/installed/mismatch/unversioned/branch_changed/invalid
	Kind          string        `json:"kind"`  // package/clone/submodule
	Path          string        `json:"path"`  // manifest-relative
	FullPath      string        `json:"full_path"`
	LocalVersion  string        `json:"local_version"`
	PinnedVersion string        `json:"pinned_version"`
	Tag           string        `json:"tag"`
	Commit        string        `json:"commit"`      // pinned branch-package HEAD sha; "" for non-pinned entries
	LiveBranch    string        `json:"live_branch"` // git checkout's current branch; "" for non-git entries
	URL           string        `json:"url"`
	Lock          bool          `json:"lock"`   // pinned: no update alerts, never bulk-updated
	Update        string        `json:"update"` // unknown/current/available
	LatestTag     string        `json:"latest_tag"`
	MissingDeps   []missDepJSON `json:"missing_deps"`
}

// missDepJSON is one unsatisfied dependency declared by an installed addon.
type missDepJSON struct {
	RepoID string `json:"repo_id"`
	Tag    string `json:"tag"`
	URL    string `json:"url"`
}

// kindLabel renders an addon.Kind as a readable label (the empty package kind
// becomes "package").
func kindLabel(k addon.Kind) string {
	if k == addon.KindPackage {
		return "package"
	}
	return string(k)
}

// printListJSON marshals the inspected statuses as a JSON array to stdout. It's
// local-only unless --check-updates is set, in which case each addon's update state
// is resolved over the network. The array is always valid JSON ("[]" when empty).
func printListJSON(statuses []addon.Status, projectRoot string) error {
	manifestAddons := make([]addon.Addon, 0, len(statuses))
	for _, s := range statuses {
		manifestAddons = append(manifestAddons, s.Addon)
	}

	// Resolve every addon's update state up front and concurrently (only when asked,
	// since it's network-bound); the per-entry loop below reads the cached result.
	var checks map[string]addon.UpdateInfo
	if checkUpdatesFlag {
		checks = addon.CheckUpdates(context.Background(), statuses)
	}

	entries := make([]listEntryJSON, 0, len(statuses))
	for _, s := range statuses {
		deps := make([]missDepJSON, 0)
		if missing, err := addon.MissingDeps(s.Addon, projectRoot, manifestAddons); err == nil {
			for _, d := range missing {
				deps = append(deps, missDepJSON{RepoID: d.RepoID, Tag: d.Tag, URL: d.RepoURL})
			}
		}

		// Lock is a local fact (no network), so report "locked" regardless of
		// --check-updates; otherwise resolve the update state over the network only when
		// --check-updates is set.
		update, latestTag := addon.UpdateUnknown.String(), ""
		switch {
		case s.Addon.Lock:
			update = addon.UpdateLocked.String()
		case checkUpdatesFlag:
			info := checks[s.Addon.Name]
			update, latestTag = info.State.String(), info.LatestTag
		}

		entries = append(entries, listEntryJSON{
			Name:          s.Addon.Name,
			State:         s.State.String(),
			Kind:          kindLabel(s.Addon.Kind),
			Path:          s.Addon.Path,
			FullPath:      s.FullPath,
			LocalVersion:  s.LocalVersion,
			PinnedVersion: s.Addon.Version,
			Tag:           s.Addon.Tag,
			Commit:        s.Addon.Commit,
			LiveBranch:    s.LiveBranch,
			URL:           s.Addon.URL,
			Lock:          s.Addon.Lock,
			Update:        update,
			LatestTag:     latestTag,
			MissingDeps:   deps,
		})
	}

	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
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

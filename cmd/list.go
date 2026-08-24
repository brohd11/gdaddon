package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/brohd11/gdaddon/internal/addon"

	"github.com/spf13/cobra"
)

var (
	listJSON    bool
	listUpdates bool
)

var listCmd = &cobra.Command{
	Use:   "list [project_root]",
	Short: "Print the manifest's install status without changing anything",
	Long: `List inspects the project's addon manifest and reports each entry's state
(missing / installed / mismatch / unversioned / branch_changed / invalid) with its
local and pinned versions.

Everything it reads is local and instant, so it never touches the network — unless
--updates is passed, which additionally checks each addon for a newer release.

  --json      emit a JSON array for tools to parse instead of the table
  --updates   also resolve each addon's update state (network)

The project root defaults to the git toplevel, else the current directory.`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE:          runListCmd,
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "print status as JSON for machine consumption")
	listCmd.Flags().BoolVar(&listUpdates, "updates", false, "also check each addon for a newer release (network)")
	rootCmd.AddCommand(listCmd)
}

func runListCmd(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveRootArg(args)
	if err != nil {
		return err
	}
	return runList(projectRoot, listJSON, listUpdates)
}

// runList is the read-only path: discover the manifest, inspect it, and print each
// addon's local state and version. The flags are parameters rather than package globals
// so the printers have exactly one source of truth for them.
func runList(projectRoot string, asJSON, withUpdates bool) error {
	manifest, err := discoverManifest(projectRoot)
	if err != nil {
		return err
	}
	statuses, err := addon.Inspect(manifest, projectRoot)
	if err != nil {
		return err
	}
	if asJSON {
		return printListJSON(statuses, projectRoot, withUpdates)
	}
	if len(statuses) == 0 {
		fmt.Println("No addons found in YAML.")
		return nil
	}
	printListTable(statuses, withUpdates)
	return nil
}

// printListTable renders the human-readable status table. With withUpdates it grows an
// update= column from the same network check the JSON path uses — the flag used to be
// silently ignored without --json, which is exactly the kind of surprise this surface
// is meant to be free of.
func printListTable(statuses []addon.Status, withUpdates bool) {
	checks := resolveUpdateStates(statuses, withUpdates)
	for _, s := range statuses {
		ver := s.Addon.Version
		if ver == "" {
			ver = "-"
		}
		local := s.LocalVersion
		if local == "" {
			local = "-"
		}
		line := fmt.Sprintf("%-12s %-24s local=%s pinned=%s", s.State.String(), s.Addon.Name, local, ver)
		if withUpdates {
			state, latest := updateStateFor(s, checks, withUpdates)
			line += " update=" + state
			if latest != "" {
				line += " (" + latest + ")"
			}
		}
		fmt.Println(line)
	}
}

// listEntryJSON is the stable, machine-parseable shape of one addon's status,
// emitted by `gdaddon list --json` for the GDScript side to consume.
type listEntryJSON struct {
	Name          string `json:"name"`
	State         string `json:"state"` // missing/installed/mismatch/unversioned/branch_changed/invalid
	Kind          string `json:"kind"`  // package/clone/submodule
	Path          string `json:"path"`  // manifest-relative
	FullPath      string `json:"full_path"`
	LocalVersion  string `json:"local_version"`
	PinnedVersion string `json:"pinned_version"`
	Tag           string `json:"tag"`
	Commit        string `json:"commit"`      // pinned branch-package HEAD sha; "" for non-pinned entries
	LiveBranch    string `json:"live_branch"` // git checkout's current branch; "" for non-git entries
	URL           string `json:"url"`
	Lock          bool   `json:"lock"`          // pinned: no update alerts, never bulk-updated
	IsDependency  bool   `json:"is_dependency"` // auto-added because another plugin declares it as a dep
	Orphan        bool   `json:"orphan"`        // an is_dependency entry nothing installed still requires
	Update        string `json:"update"`        // unknown/current/available
	LatestTag     string `json:"latest_tag"`
	// Ahead/Behind are a git checkout's divergence from its upstream (0 for everything
	// else). They're read locally from the remote-tracking refs, so they cost nothing —
	// but for the same reason they're only as current as the last `git fetch` in that
	// checkout; gdaddon never fetches on a list.
	Ahead       int           `json:"ahead"`
	Behind      int           `json:"behind"`
	MissingDeps []missDepJSON `json:"missing_deps"`
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

// resolveUpdateStates resolves every addon's update state up front and concurrently
// (only when asked, since it's network-bound), for the per-entry loops to read.
func resolveUpdateStates(statuses []addon.Status, withUpdates bool) map[string]addon.UpdateInfo {
	if !withUpdates {
		return nil
	}
	return addon.CheckUpdates(context.Background(), statuses)
}

// updateStateFor is one addon's update state and latest tag. Lock is a local fact (no
// network), so a locked entry reports "locked" with or without --updates; everything
// else stays "unknown" until the network check has run.
func updateStateFor(s addon.Status, checks map[string]addon.UpdateInfo, withUpdates bool) (state, latestTag string) {
	switch {
	case s.Addon.Lock:
		return addon.UpdateLocked.String(), ""
	case withUpdates:
		info := checks[s.Addon.Name]
		return info.State.String(), info.LatestTag
	}
	return addon.UpdateUnknown.String(), ""
}

// printListJSON marshals the inspected statuses as a JSON array to stdout. It's
// local-only unless withUpdates is set, in which case each addon's update state
// is resolved over the network. The array is always valid JSON ("[]" when empty).
func printListJSON(statuses []addon.Status, projectRoot string, withUpdates bool) error {
	manifestAddons := make([]addon.Addon, 0, len(statuses))
	for _, s := range statuses {
		manifestAddons = append(manifestAddons, s.Addon)
	}

	checks := resolveUpdateStates(statuses, withUpdates)

	// Orphan state is local (which is_dependency entries nothing installed still needs),
	// computed once over the whole set and read per-entry below.
	orphans := addon.OrphanDeps(statuses)

	entries := make([]listEntryJSON, 0, len(statuses))
	for _, s := range statuses {
		deps := make([]missDepJSON, 0)
		if missing, err := addon.MissingDeps(s.Addon, projectRoot, manifestAddons); err == nil {
			for _, d := range missing {
				deps = append(deps, missDepJSON{RepoID: d.RepoID, Tag: d.Tag, URL: d.RepoURL})
			}
		}

		update, latestTag := updateStateFor(s, checks, withUpdates)

		// Divergence only means anything for a checkout that's actually on disk; everything
		// else reports 0/0. Local read, so it needs no --updates gate.
		var sync addon.GitSync
		if s.Addon.IsGitWorkdir() && s.Present() {
			sync = addon.GitSyncStatus(s.FullPath)
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
			IsDependency:  s.Addon.Dependency,
			Orphan:        orphans[s.Addon.Name],
			Update:        update,
			LatestTag:     latestTag,
			Ahead:         sync.Ahead,
			Behind:        sync.Behind,
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

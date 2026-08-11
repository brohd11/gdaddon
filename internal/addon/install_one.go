package addon

import (
	"context"
	"fmt"
	"path/filepath"

	"gdaddon/internal/source"
)

// InstallOneOpts describes a single targeted install: one addon, recorded in the
// manifest and installed, optionally followed by its own dependency closure.
type InstallOneOpts struct {
	ManifestPath string
	ProjectRoot  string
	// Entry is the addon to install with its Name/URL/Tag/Kind already resolved by the
	// caller (a release asset url + tag, or a canonical .git url + branch for a clone).
	// Path is left to the installer to derive unless the manifest already pins one.
	Entry  Addon
	Deps   bool // also install the dependency closure this addon declares
	Report Reporter
}

// InstallOneResult is where the entry landed, plus every dependency installed on its
// behalf (empty when Deps was false or the addon declares none).
type InstallOneResult struct {
	Name    string
	Path    string
	Version string
	Deps    []InstallOutcome
}

// InstallOne records one addon in the manifest and installs it — the targeted
// counterpart to InstallAll, which applies a whole manifest. It upserts the entry (so
// re-installing an already-tracked repo re-pins it rather than erroring on the
// duplicate), installs it, pins the resolved path/version/kind back, and — when Deps
// is set — walks the dependency closure rooted at it via InstallDepsFor.
//
// It is front-end agnostic: the CLI's `gdaddon install <owner/repo>` is the first
// caller, and a per-addon TUI action can use it unchanged.
func InstallOne(ctx context.Context, o InstallOneOpts) (InstallOneResult, error) {
	report := o.Report
	if report == nil {
		report = func(string, ...any) {}
	}

	// The repo may already be tracked under a different name than the one derived from
	// the url; the manifest's name wins so the install re-pins that entry instead of
	// adding a second one for the same repo.
	name := o.Entry.Name
	if entries, err := Parse(o.ManifestPath); err == nil {
		if e, ok := FindByRepo(entries, o.Entry.URL); ok {
			name = e.Name
		}
	}
	o.Entry.Name = name

	if err := UpsertEntry(o.ManifestPath, o.Entry); err != nil {
		return InstallOneResult{}, err
	}

	// Re-read the entry the manifest now holds rather than installing the bare input:
	// that picks up a previously pinned (possibly relocated) path and any suppress_deps
	// the user set, both of which change what Install does.
	entry, err := entryNamed(o.ManifestPath, name)
	if err != nil {
		return InstallOneResult{}, err
	}

	res, err := Install(ctx, entry, o.ProjectRoot, report)
	if err != nil {
		return InstallOneResult{}, err
	}

	if res.Path != "" {
		// A clone records no version: it tracks a branch, so the plugin.cfg version it
		// happens to carry right now is not a pin (and Inspect ignores it for a git
		// workdir anyway). Same rule as the TUI's pinInstall.
		version := res.Version
		if o.Entry.Kind == KindClone {
			version = ""
		}
		if err := UpdateEntry(o.ManifestPath, name, "", res.Path, version, ""); err != nil {
			return InstallOneResult{}, err
		}
		entry.Path, entry.Version = res.Path, version
	}
	// Always write the kind so a package install over a former clone clears the stale
	// kind line, and clear any commit pin a previous branch-package install left — this
	// install came from a release tag or a live clone, neither of which is sha-pinned.
	if err := SetKind(o.ManifestPath, name, o.Entry.Kind); err != nil {
		return InstallOneResult{}, err
	}
	if err := SetCommit(o.ManifestPath, name, ""); err != nil {
		return InstallOneResult{}, err
	}

	// A clone with no branch named landed on whatever the remote's default is. Record
	// it: a clone entry whose tag doesn't match the checked-out branch reads as
	// branch-drifted (StateBranchChanged) on every subsequent inspect.
	if o.Entry.Kind == KindClone && o.Entry.Tag == "" && res.Path != "" {
		if branch := CurrentBranch(filepath.Join(o.ProjectRoot, res.Path)); branch != "" {
			if err := UpdateEntry(o.ManifestPath, name, "", "", "", branch); err != nil {
				return InstallOneResult{}, err
			}
			entry.Tag = branch
		}
	}

	out := InstallOneResult{Name: name, Path: res.Path, Version: res.Version}
	if !o.Deps {
		return out, nil
	}
	deps, err := InstallDepsFor(ctx, o.ManifestPath, entry, o.ProjectRoot, report)
	out.Deps = deps
	return out, err
}

// InstallDepsFor installs the dependency closure rooted at one installed addon, and
// nothing else. It reads the deps that addon declares in its installed plugin.cfg,
// adds the ones the manifest doesn't satisfy (AddDepEntry, which records the
// is_dependency provenance), installs each, then recurses into what those in turn
// declare — breadth-first, deduped by canonical repo identity so a diamond resolves
// once and a cycle terminates.
//
// This is the targeted counterpart to InstallAllDeps: that one is a manifest-wide
// fixed-point loop that installs *every* entry and imports *every* declared dep, which
// is the right thing for "set up this project" but the wrong thing for "install this
// one plugin". An entry unrelated to the root addon is never touched here.
func InstallDepsFor(ctx context.Context, manifestPath string, root Addon, baseDir string, report Reporter) ([]InstallOutcome, error) {
	if report == nil {
		report = func(string, ...any) {}
	}

	// Seed the visited set with the root's own identity so a plugin that (directly or
	// transitively) declares itself doesn't reinstall itself.
	seen := make(map[string]bool)
	if id, err := source.RepoID(root.URL); err == nil {
		seen[id] = true
	}

	var outcomes []InstallOutcome
	queue := []Addon{root}
	for depth := 0; len(queue) > 0; depth++ {
		if depth >= maxDepRounds {
			report("Dependency resolution stopped after %d levels.", maxDepRounds)
			break
		}
		var next []Addon
		for _, a := range queue {
			// A not-installed addon has no plugin.cfg on disk, so it declares nothing.
			if a.Path == "" {
				continue
			}
			deps, err := Dependencies(filepath.Join(baseDir, a.Path))
			if err != nil || len(deps) == 0 {
				continue
			}
			suppressed := stringSet(a.SuppressDeps)
			for _, d := range deps {
				if suppressed[d.RepoID] || seen[d.RepoID] {
					continue
				}
				seen[d.RepoID] = true
				entry, outcome, ok := ensureDep(ctx, manifestPath, d, baseDir, report)
				if !ok {
					continue
				}
				// Also mark what it actually resolved to: an upstream-renamed repo
				// records a different identity than the spec declared, and something
				// else in the graph may declare it under that newer name.
				if id, err := source.RepoID(entry.URL); err == nil {
					seen[id] = true
				}
				if outcome != nil {
					outcomes = append(outcomes, *outcome)
				}
				next = append(next, entry)
			}
		}
		queue = next
	}
	return outcomes, nil
}

// ensureDep brings one declared dependency up to satisfied: it records the entry when
// the manifest lacks one (or re-pins it when the recorded tag is verifiably older than
// required) and installs it unless an installed, satisfying copy is already there. The
// returned entry is enqueued by the caller so the dependency's own deps are visited
// either way; outcome is nil when nothing was installed. ok is false when the dep
// couldn't be resolved at all — reported, then skipped.
func ensureDep(ctx context.Context, manifestPath string, d Dependency, baseDir string, report Reporter) (Addon, *InstallOutcome, bool) {
	statuses, err := Inspect(manifestPath, baseDir)
	if err != nil {
		report("  -> Could not read the manifest for %s: %v", d.RepoID, err)
		return Addon{}, nil, false
	}
	// entryName is what this dependency is (or would be) recorded as. It's the fallback
	// identity because a repo renamed upstream records its *new* id — the release asset
	// url's — which no longer matches the id the declared spec parses to.
	entryName := DeriveName(d.RepoURL)
	st, present := statusesByRepo(statuses)[d.RepoID]
	if !present {
		st, present = statusNamed(statuses, entryName)
	}

	switch {
	case present && st.Present() && depSatisfied(d, st.Addon.Tag):
		report("  -> %s is already installed. Skipping...", st.Addon.Name)
		return st.Addon, nil, true

	case !present:
		// The ctx here only bounds the release-listing lookup, matching importDeps.
		lookup, cancel := context.WithTimeout(ctx, DepsResolveTimeout)
		name, added, err := AddDepEntry(lookup, manifestPath, d, true)
		cancel()
		if err != nil {
			report("  -> Could not add %s: %v", d.RepoID, err)
			return Addon{}, nil, false
		}
		if !added {
			report("  -> Skipping %s: no asset for %s", d.RepoID, d.Tag)
			return Addon{}, nil, false
		}
		if d.Tag == "" {
			report("  -> Added %s (no version)", name)
		} else {
			report("  -> Added %s %s", name, d.Tag)
		}
		entryName = name

	case d.Tag != "":
		// Present but verifiably older than required: re-pin it at the required tag.
		// UpsertEntry (not AddDepEntry) because the entry already exists.
		lookup, cancel := context.WithTimeout(ctx, DepsResolveTimeout)
		asset, ok := ResolveDepAsset(lookup, d)
		cancel()
		if !ok {
			report("  -> Skipping %s: no asset for %s", d.RepoID, d.Tag)
			return st.Addon, nil, false
		}
		if err := UpsertEntry(manifestPath, Addon{Name: st.Addon.Name, URL: asset.URL, Tag: d.Tag}); err != nil {
			report("  -> Could not re-pin %s: %v", st.Addon.Name, err)
			return st.Addon, nil, false
		}
		report("  -> Re-pinned %s %s → %s", st.Addon.Name, st.Addon.Tag, d.Tag)
		entryName = st.Addon.Name
	}

	// By name, not by repo: the entry that was just written records the url the tag
	// resolved to, whose repo id can differ from the declared spec's (see entryName).
	entry, err := entryNamed(manifestPath, entryName)
	if err != nil {
		report("  -> %v", err)
		return Addon{}, nil, false
	}

	res, err := Install(ctx, entry, baseDir, report)
	if err != nil {
		report("  -> [%s] Error: %v", entry.Name, err)
		return entry, nil, false
	}
	if res.Path == "" {
		// A multi-addon package with nothing to pin; still visit its deps.
		return entry, nil, true
	}
	if err := UpdateEntry(manifestPath, entry.Name, "", res.Path, res.Version, ""); err != nil {
		report("  -> Could not pin %s: %v", entry.Name, err)
	}
	outcome := InstallOutcome{
		Name: entry.Name, URL: entry.URL, PriorPath: entry.Path, Path: res.Path, Version: res.Version,
	}
	entry.Path, entry.Version = res.Path, res.Version
	return entry, &outcome, true
}

// depSatisfied reports whether an entry on installedTag meets the dependency, using
// the same rule as MissingDeps/DepStatuses: a tagless dep is satisfied by presence,
// and a tag pair that can't be compared (a date stamp, a branch entry with no tag) is
// trusted rather than treated as a miss.
func depSatisfied(d Dependency, installedTag string) bool {
	if d.Tag == "" {
		return true
	}
	sat, verified := d.SatisfiedByTag(installedTag)
	return !verified || sat
}

// statusNamed returns the inspected status of the entry with the given name.
func statusNamed(statuses []Status, name string) (Status, bool) {
	for _, s := range statuses {
		if s.Addon.Name == name {
			return s, true
		}
	}
	return Status{}, false
}

// entryNamed returns the manifest entry with the given name.
func entryNamed(manifestPath, name string) (Addon, error) {
	entries, err := Parse(manifestPath)
	if err != nil {
		return Addon{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			return e, nil
		}
	}
	return Addon{}, fmt.Errorf("addon %q not found in %s", name, filepath.Base(manifestPath))
}

package addon

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/brohd11/gdaddon/internal/source"
)

// InstallOneOpts describes a single targeted install: one addon, recorded in the
// manifest and installed, optionally followed by its own dependency closure.
type InstallOneOpts struct {
	ManifestPath string
	ProjectRoot  string
	// Entry is the addon to install with its Name/URL/Tag/Kind already resolved by the
	// caller (a release asset url + tag, or a canonical .git url + branch for a clone).
	// Path is left to the installer to derive unless the manifest already pins one.
	Entry Addon
	Deps  bool // also install the dependency closure this addon declares
	// ConfirmDep, when set, vets every dependency before it is recorded or downloaded;
	// nil installs the whole closure unattended. The named addon itself is never
	// vetted — the user asked for that one by name.
	ConfirmDep DepConfirmer
	Report     Reporter
}

// InstallOneResult is where the entry landed, plus every dependency installed on its
// behalf (empty when Deps was false or the addon declares none).
type InstallOneResult struct {
	Name string
	// Display is the entry's label once installed — the addon's own name when it
	// declares one — so a caller can report what it installed rather than the key.
	Display string
	Path    string
	Version string
	Deps    []InstallOutcome
}

// Label is the installed entry's human-facing name, on the same rule as Addon.Label.
func (r InstallOneResult) Label() string {
	return Addon{Name: r.Name, Display: r.Display}.Label()
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
		if err := AdoptName(o.ManifestPath, entry, res); err != nil {
			report("  -> Could not record the declared name for %s: %v", entry.Label(), err)
		}
		if entry, err = pinInstalled(o.ManifestPath, o.ProjectRoot, entry, res); err != nil {
			return InstallOneResult{}, err
		}
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

	out := InstallOneResult{Name: name, Display: displayOf(entry, res), Path: res.Path, Version: res.Version}
	if !o.Deps {
		return out, nil
	}
	deps, err := InstallDepsFor(ctx, o.ManifestPath, entry, o.ProjectRoot, o.ConfirmDep, report)
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
//
// confirm, when non-nil, vets each dependency before it is recorded or downloaded. A
// declined dependency is dropped from the queue, so its own declared dependencies are
// never visited either — refusing a package refuses the subtree under it, which is the
// whole point of asking. A confirmer error (ErrDepAborted for a user quit) stops the
// walk and is returned alongside whatever was already installed.
func InstallDepsFor(ctx context.Context, manifestPath string, root Addon, baseDir string, confirm DepConfirmer, report Reporter) ([]InstallOutcome, error) {
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
				entry, outcome, ok, err := ensureDep(ctx, manifestPath, d, a.Name, baseDir, confirm, report)
				if err != nil {
					return outcomes, err
				}
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
// couldn't be resolved, was declined, or failed to install — reported, then skipped.
//
// It runs as classify → confirm → commit. The classification resolves the dependency
// (the network call a confirmer needs to be shown a real download url) but writes
// nothing, so declining leaves no manifest entry stranded ahead of an install that
// never happened — which is exactly what the old shape, where each branch wrote before
// installing, could not offer. A non-nil error is the confirmer's and aborts the walk.
func ensureDep(ctx context.Context, manifestPath string, d Dependency, declaredBy, baseDir string, confirm DepConfirmer, report Reporter) (Addon, *InstallOutcome, bool, error) {
	statuses, err := Inspect(manifestPath, baseDir)
	if err != nil {
		report("  -> Could not read the manifest for %s: %v", d.RepoID, err)
		return Addon{}, nil, false, nil
	}
	// entryName is what this dependency is (or would be) recorded as — the same identity
	// depIndex falls back to when the repo was renamed upstream.
	entryName := EntryKey(d.RepoURL)
	var st Status
	i := newDepIndex(addonsOf(statuses)).find(d)
	present := i >= 0
	if present {
		st = statuses[i]
	}

	req := DepRequest{Dep: d, DeclaredBy: declaredBy, EntryName: entryName}
	// commit performs the classification's manifest write once the dependency is
	// confirmed, reporting and returning false on failure. The default is the branch
	// that needs no write at all: an entry already recorded, just not on disk.
	commit := func() bool { return true }

	switch {
	case present && st.Present() && depSatisfied(d, st.Addon.Tag):
		report("  -> %s is already installed. Skipping...", st.Addon.Label())
		return st.Addon, nil, true, nil

	case !present:
		// d is reassigned: an `@latest` spec comes back carrying the tag it resolved to,
		// which is what the confirmer is shown and what gets written.
		d, asset, ok := resolveDepAssetTimed(ctx, d)
		if !ok {
			report("  -> Skipping %s: no asset for %s", d.RepoID, d.Tag)
			return Addon{}, nil, false, nil
		}
		req.Dep = d
		req.Action = DepAdd
		req.AssetURL = depDownloadURL(d, asset)
		commit = func() bool {
			written, _, err := writeDepEntry(manifestPath, d, asset, true, true)
			if err != nil {
				report("  -> Could not add %s: %v", d.RepoID, err)
				return false
			}
			report("  -> Added %s %s", written.Name, DepLabel(written.Kind, written.Tag))
			entryName = written.Name
			return true
		}

	case d.Tag != "" && !d.IsClone():
		// Present but not satisfying (older than required, or recorded yet absent from
		// disk): re-pin it at the required tag. UpsertEntry (not AddDepEntry) because
		// the entry already exists. A clone never reaches here — depSatisfied treats a
		// present entry as satisfying one — and must not, since re-pinning would rewrite
		// a live checkout's entry to a release asset; the guard says so rather than
		// leaving it to that invariant holding at a distance.
		d, asset, ok := resolveDepAssetTimed(ctx, d)
		if !ok {
			report("  -> Skipping %s: no asset for %s", d.RepoID, d.Tag)
			return st.Addon, nil, false, nil
		}
		// The write is the same either way; the label distinguishes a genuine version
		// bump from re-fetching files that went missing under an already-fine tag.
		req.Dep = d
		req.Action = DepRepin
		if depSatisfied(d, st.Addon.Tag) {
			req.Action = DepReinstall
		}
		req.AssetURL, req.EntryName, req.LocalTag = asset.URL, st.Addon.Name, st.Addon.Tag
		commit = func() bool {
			if err := UpsertEntry(manifestPath, Addon{Name: st.Addon.Name, URL: asset.URL, Tag: d.Tag}); err != nil {
				report("  -> Could not re-pin %s: %v", st.Addon.Label(), err)
				return false
			}
			report("  -> Re-pinned %s %s → %s", st.Addon.Label(), st.Addon.Tag, d.Tag)
			entryName = st.Addon.Name
			return true
		}

	default:
		// Recorded but not on disk, with no tag to re-resolve: install what's recorded.
		req.Action = DepReinstall
		req.AssetURL, req.EntryName, req.LocalTag = st.Addon.URL, st.Addon.Name, st.Addon.Tag
		entryName = st.Addon.Name
	}

	ok, err := allowDep(confirm, req)
	if err != nil {
		return Addon{}, nil, false, err
	}
	if !ok {
		report("  -> Skipped %s (declined).", d.RepoID)
		return Addon{}, nil, false, nil
	}
	if !commit() {
		return Addon{}, nil, false, nil
	}

	// By name, not by repo: the entry that was just written records the url the tag
	// resolved to, whose repo id can differ from the declared spec's (see entryName).
	entry, err := entryNamed(manifestPath, entryName)
	if err != nil {
		report("  -> %v", err)
		return Addon{}, nil, false, nil
	}

	res, err := Install(ctx, entry, baseDir, report)
	if err != nil {
		report("  -> [%s] Error: %v", entry.Label(), err)
		return entry, nil, false, nil
	}
	if res.Path == "" {
		// A multi-addon package with nothing to pin; still visit its deps.
		return entry, nil, true, nil
	}
	if err := AdoptName(manifestPath, entry, res); err != nil {
		report("  -> Could not record the declared name for %s: %v", entry.Label(), err)
	}
	outcome := InstallOutcome{
		Name: entry.Name, Display: displayOf(entry, res), URL: entry.URL,
		PriorPath: entry.Path, Path: res.Path, Version: res.Version,
	}
	pinned, err := pinInstalled(manifestPath, baseDir, entry, res)
	if err != nil {
		report("  -> Could not pin %s: %v", entry.Label(), err)
		return entry, &outcome, true, nil
	}
	return pinned, &outcome, true, nil
}

// pinInstalled records what an install produced on the entry's manifest row: the path it
// landed at, and its version — except for a clone, which records none, because it tracks
// a branch and the plugin.cfg version it happens to carry right now is not a pin (Inspect
// ignores it for a git workdir anyway). A clone installed without a branch named also has
// the branch it landed on written back, since a clone entry whose tag doesn't match the
// checked-out branch reads as branch-drifted (StateBranchChanged) on every later inspect.
//
// Three sites install and then pin — InstallOne, ensureDep and InstallAll — and these two
// clone rules used to live only in the first, so a clone installed through either of the
// others recorded a version it does not have and read as drifted forever. They get one
// home rather than a copy each.
func pinInstalled(manifestPath, projectRoot string, entry Addon, res InstallResult) (Addon, error) {
	version := res.Version
	if entry.Kind == KindClone {
		version = ""
	}
	if err := UpdateEntry(manifestPath, entry.Name, "", res.Path, version, ""); err != nil {
		return entry, err
	}
	entry.Path, entry.Version = res.Path, version

	if entry.Kind == KindClone && entry.Tag == "" {
		if branch := CurrentBranch(filepath.Join(projectRoot, res.Path)); branch != "" {
			if err := UpdateEntry(manifestPath, entry.Name, "", "", "", branch); err != nil {
				return entry, err
			}
			entry.Tag = branch
		}
	}
	return entry, nil
}

// resolveDepAssetTimed resolves a tagged dependency's asset under DepsResolveTimeout —
// the ctx bounds only the release listing, matching importDeps. A clone (which checks out
// a branch) and a tagless dependency (added repo-only) have no release to look up, so
// both resolve trivially. Like ResolveDepAsset it hands the dependency back, carrying the
// tag an `@latest` spec resolved to.
func resolveDepAssetTimed(ctx context.Context, d Dependency) (Dependency, source.Asset, bool) {
	if !needsDepAsset(d) {
		return d, source.Asset{}, true
	}
	lookup, cancel := context.WithTimeout(ctx, DepsResolveTimeout)
	defer cancel()
	return ResolveDepAsset(lookup, d)
}

// depDownloadURL is the url a resolved dependency would actually be fetched from, for
// display to a DepConfirmer: the release asset for a tagged dep, else the repo url a
// clone or a tagless one is cloned from (what writeDepEntry records).
func depDownloadURL(d Dependency, asset source.Asset) string {
	if d.Tag == "" || d.IsClone() {
		return NormalizeRepoURL(d.RepoURL)
	}
	return asset.URL
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

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
	// Entry is the addon to install, resolved by the caller (a release asset and tag, or a .git
	// url and branch). Path is derived unless the manifest pins one.
	Entry Addon
	Deps  bool // also install the dependency closure this addon declares
	// ConfirmDep vets every dependency before it is recorded or downloaded (nil installs all).
	// The named addon itself is never vetted.
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

// InstallOne records one addon in the manifest and installs it: it upserts the entry (so a
// tracked repo is re-pinned), installs, pins the result, and with Deps walks its dependency
// closure (InstallDepsFor). The CLI's install uses it; a TUI action could too.
func InstallOne(ctx context.Context, o InstallOneOpts) (InstallOneResult, error) {
	report := o.Report
	if report == nil {
		report = func(string, ...any) {}
	}

	// The repo may already be tracked under another name; the manifest's name wins, so the
	// existing entry is re-pinned rather than duplicated.
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

	// Re-read the stored entry: its pinned path and suppress_deps change what Install does.
	entry, err := entryNamed(o.ManifestPath, name)
	if err != nil {
		return InstallOneResult{}, err
	}

	res, err := Install(ctx, entry, o.ProjectRoot, report)
	if err != nil {
		return InstallOneResult{}, err
	}

	if res.Path != "" {
		if entry, _, err = recordInstall(o.ManifestPath, o.ProjectRoot, entry, res, func(err error) {
			report("  -> Could not record the declared name for %s: %v", entry.Label(), err)
		}); err != nil {
			return InstallOneResult{}, err
		}
	}
	// Always write the kind (clearing a stale clone kind) and clear any commit pin: this
	// install is neither.
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

// InstallDepsFor installs only the dependency closure of one installed addon: it adds the
// dependencies the manifest does not satisfy, installs them and recurses breadth-first,
// deduplicated by repo identity (diamonds resolve once, cycles end). Unlike
// InstallAllDeps, unrelated entries are untouched. confirm vets each dependency; a decline
// also skips its subtree, and an error stops the walk.
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
				// Also mark the identity it resolved to: an upstream rename records a different id that
				// something else may declare.
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

// ensureDep brings one dependency to satisfied: it records it (or re-pins an older one)
// and installs it unless a satisfying copy is present. The returned entry is enqueued so
// its dependencies are visited; outcome is nil when nothing was installed, ok false when
// it could not be resolved, was declined, or failed. It classifies (resolving the url the
// confirmer shows) before writing anything, so a decline leaves no entry behind. An error
// is the confirmer's and aborts.
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
	// commit writes the classification's manifest change after confirmation; the default is an
	// entry already recorded that just needs installing.
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
		// Present but not satisfying: re-pin at the required tag (UpsertEntry). Clones never get
		// here (any present entry satisfies them), and must not, since this would turn a checkout
		// into a release asset.
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
	pinned, outcome, err := recordInstall(manifestPath, baseDir, entry, res, func(err error) {
		report("  -> Could not record the declared name for %s: %v", entry.Label(), err)
	})
	if err != nil {
		report("  -> Could not pin %s: %v", entry.Label(), err)
		return entry, &outcome, true, nil
	}
	return pinned, &outcome, true, nil
}

// recordInstall writes an install back to the manifest: the declared name (a failure is
// passed to nameErr, not returned) and the pinned path and version (see pinInstalled).
// The outcome is filled in even when pinning fails.
func recordInstall(manifestPath, projectRoot string, entry Addon, res InstallResult,
	nameErr func(error)) (Addon, InstallOutcome, error) {
	if err := AdoptName(manifestPath, entry, res); err != nil {
		nameErr(err)
	}
	outcome := InstallOutcome{
		Name: entry.Name, Display: displayOf(entry, res), URL: entry.URL,
		PriorPath: entry.Path, Path: res.Path, Version: res.Version,
	}
	pinned, err := pinInstalled(manifestPath, projectRoot, entry, res)
	return pinned, outcome, err
}

// pinInstalled records an install's path and version on its entry. A clone records no
// version (it tracks a branch), and a clone installed without a named branch records the
// one it landed on, or it would read as drifted. Every install path pins through here.
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

// resolveDepAssetTimed resolves a tagged dependency's asset under DepsResolveTimeout;
// clones and tagless dependencies need no lookup. It returns d with @latest resolved.
func resolveDepAssetTimed(ctx context.Context, d Dependency) (Dependency, source.Asset, bool) {
	if !needsDepAsset(d) {
		return d, source.Asset{}, true
	}
	lookup, cancel := context.WithTimeout(ctx, DepsResolveTimeout)
	defer cancel()
	return ResolveDepAsset(lookup, d)
}

// depDownloadURL is what a resolved dependency would be fetched from, for the confirmer:
// the release asset, or the repo url for clones and tagless deps.
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

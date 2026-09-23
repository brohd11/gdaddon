package addon

import (
	"context"
	"time"

	"github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/source"

	"github.com/brohd11/goutil/strutil"
)

// DepsResolveTimeout caps a batch of release lookups, shared by the recursive install and
// the TUI.
const DepsResolveTimeout = 30 * time.Second

// maxDepRounds bounds the install/import loop against cyclic or unresolvable graphs.
const maxDepRounds = 10

// InstallAllDeps installs the manifest, then repeatedly imports and installs dependencies
// declared by installed addons until a round adds nothing (or the cap is hit). confirm
// vets only newly discovered dependencies; entries already in the manifest install
// without asking. A confirmer error stops the loop.
func InstallAllDeps(ctx context.Context, manifestPath, baseDir string, confirm DepConfirmer, report Reporter) ([]InstallOutcome, error) {
	var outcomes []InstallOutcome
	// Remember declines across rounds, or each round would offer them again.
	declined := make(map[string]bool)
	for round := 1; round <= maxDepRounds; round++ {
		statuses, err := Inspect(manifestPath, baseDir)
		if err != nil {
			return outcomes, err
		}
		got, err := InstallAll(ctx, manifestPath, statuses, baseDir, report)
		if err != nil {
			return outcomes, err
		}
		outcomes = append(outcomes, got...)

		added, err := importDeps(ctx, manifestPath, baseDir, confirm, declined, report)
		if err != nil {
			return outcomes, err
		}
		if added == 0 {
			return outcomes, nil
		}
		report("Added %d dependenc%s to the manifest; installing…", added, strutil.Plural(added, "y", "ies"))
	}
	report("Dependency resolution stopped after %d rounds.", maxDepRounds)
	return outcomes, nil
}

// importDeps adds to the manifest every declared dependency it can resolve that the
// manifest does not satisfy, returning how many were added. confirm vets each after
// resolving; declines are recorded in declined. A confirmer error aborts.
func importDeps(parent context.Context, manifestPath, baseDir string, confirm DepConfirmer, declined map[string]bool, report Reporter) (int, error) {
	statuses, err := Inspect(manifestPath, baseDir)
	if err != nil {
		return 0, nil
	}
	manifest, err := Parse(manifestPath)
	if err != nil {
		return 0, nil
	}

	// Deduplicate by repo identity in declaration order (the confirm order is user-visible).
	var missing []Dependency
	declaredBy := make(map[string]string)
	seen := make(map[string]bool)
	for _, s := range statuses {
		if !s.Present() {
			continue
		}
		deps, err := MissingDeps(s.Addon, baseDir, manifest)
		if err != nil {
			continue
		}
		for _, d := range deps {
			if seen[d.RepoID] || declined[d.RepoID] {
				continue
			}
			seen[d.RepoID] = true
			declaredBy[d.RepoID] = s.Addon.Name
			missing = append(missing, d)
		}
	}
	if len(missing) == 0 {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(parent, DepsResolveTimeout)
	defer cancel()

	added := 0
	for _, d := range missing {
		asset, resolved := source.Asset{}, true
		if needsDepAsset(d) {
			// d is reassigned: an `@latest` spec comes back carrying the tag it resolved
			// to, and that is what gets confirmed, written and reported below.
			d, asset, resolved = ResolveDepAsset(ctx, d)
		}
		if !resolved {
			report("  -> Skipping %s: no asset for %s", d.RepoID, d.Tag)
			continue
		}

		ok, err := allowDep(confirm, DepRequest{
			Dep:        d,
			DeclaredBy: declaredBy[d.RepoID],
			Action:     DepAdd,
			EntryName:  EntryKey(d.RepoURL),
			AssetURL:   depDownloadURL(d, asset),
		})
		if err != nil {
			return added, err
		}
		if !ok {
			declined[d.RepoID] = true
			report("  -> Skipped %s (declined).", d.RepoID)
			continue
		}

		entry, ok, err := writeDepEntry(manifestPath, d, asset, resolved, false)
		if err != nil {
			report("  -> Could not add %s: %v", d.RepoID, err)
			continue
		}
		if !ok {
			report("  -> Skipping %s: no asset for %s", d.RepoID, d.Tag)
			continue
		}
		report("  -> Added %s %s", entry.Name, DepLabel(entry.Kind, entry.Tag))
		added++
	}
	return added, nil
}

// AddDepEntry resolves one dependency and adds it: a clone as a branch checkout, a tagless
// dep repo-only, a tagged dep at its release asset (see ResolveDepAsset). asDependency
// marks provenance. added is false, err nil, when a tagged dep has no asset. ctx bounds only
// the lookup.
func AddDepEntry(ctx context.Context, manifestPath string, d Dependency, asDependency bool) (entry Addon, added bool, err error) {
	asset, ok := source.Asset{}, true
	if needsDepAsset(d) {
		d, asset, ok = ResolveDepAsset(ctx, d)
	}
	return writeDepEntry(manifestPath, d, asset, ok, asDependency)
}

// needsDepAsset reports whether d needs a release asset lookup (tagged, not a clone).
func needsDepAsset(d Dependency) bool { return !d.IsClone() && d.Tag != "" }

// DepLabel describes a recorded dependency for progress and prompts, naming clones as such.
// It takes the resolved kind and ref (an @latest dep shows its actual tag).
func DepLabel(kind Kind, ref string) string {
	switch {
	case kind == KindClone && ref == "":
		return "(clone: default branch)"
	case kind == KindClone:
		return "(clone: " + ref + ")"
	case ref == "":
		return "(no version)"
	}
	return ref
}

// writeDepEntry records a dependency from an already-resolved asset, so a caller that
// resolved first (to show the confirmer the url) need not resolve twice. resolved false
// means no asset: a skip, not an error. It returns the entry written, filled in even when
// nothing was added.
func writeDepEntry(manifestPath string, d Dependency, asset source.Asset, resolved, asDependency bool) (entry Addon, added bool, err error) {
	entry = DepEntry(d, asset, asDependency)
	if d.Tag != "" && !d.IsClone() && !resolved {
		return entry, false, nil
	}
	if err := AddEntryFull(manifestPath, entry); err != nil {
		return entry, false, err
	}
	return entry, true, nil
}

// DepEntry is the manifest entry a dependency is recorded as: a clone per CloneEntry, a
// tagless dependency as its repo with no version, and a tagged one as its resolved asset.
func DepEntry(d Dependency, asset source.Asset, asDependency bool) Addon {
	name := EntryKey(d.RepoURL)
	if d.IsClone() {
		return CloneEntry(d, name, asDependency)
	}
	entry := Addon{Name: name, URL: NormalizeRepoURL(d.RepoURL), Dependency: asDependency}
	if d.Tag != "" {
		entry.URL, entry.Tag = asset.URL, d.Tag
	}
	return entry
}

// CloneEntry is the manifest entry for a clone dependency: canonical .git url, branch in
// Tag (empty for the remote default), kind clone. The CLI's clone: install uses it too.
func CloneEntry(d Dependency, name string, asDependency bool) Addon {
	return Addon{
		Name:       name,
		URL:        NormalizeRepoURL(d.RepoURL),
		Tag:        d.Tag,
		Kind:       KindClone,
		Dependency: asDependency,
	}
}

// ResolveDepAsset finds d's release (archive first, then network) and picks its asset via
// source.AutoAsset (ambiguous uploads give ok=false). It returns d with an @latest spec
// resolved to the concrete tag, which is what callers must record.
func ResolveDepAsset(ctx context.Context, d Dependency) (Dependency, source.Asset, bool) {
	// Only look in the tag-keyed archive once latest has a value, so a rolling `latest` tag is
	// never served stale.
	if !d.WantsLatest() {
		if asset, ok := archivedDepAsset(d); ok {
			return d, asset, true
		}
	}
	rel, err := ResolveVersion(ctx, d.RepoURL, d.Tag)
	if err != nil {
		return d, source.Asset{}, false
	}
	if d.WantsLatest() {
		d.Tag = rel.Tag
	}
	asset, ok := source.AutoAsset(rel)
	return d, asset, ok
}

// archivedDepAsset returns an archived asset for d's tag; ambiguity gives ok=false.
func archivedDepAsset(d Dependency) (source.Asset, bool) {
	releases, err := archive.List(d.RepoID)
	if err != nil {
		return source.Asset{}, false
	}
	for _, rel := range releases {
		if source.TagEqual(rel.Tag, d.Tag) {
			return source.AutoAsset(rel)
		}
	}
	return source.Asset{}, false
}

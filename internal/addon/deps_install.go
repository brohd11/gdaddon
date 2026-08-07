package addon

import (
	"context"
	"strings"
	"time"

	"gdaddon/internal/archive"
	"gdaddon/internal/source"
)

// DepsResolveTimeout caps a batch of release-listing fetches so a slow or unreachable
// host can't hang dependency resolution. The recursive install's rounds and the TUI's
// resolve commands share it so both cap the same way.
const DepsResolveTimeout = 30 * time.Second

// maxDepRounds bounds the install→import→install loop so an unresolvable or cyclic
// dependency graph can't spin forever. In practice a handful of rounds covers any
// realistic transitive depth.
const maxDepRounds = 10

// InstallAllDeps installs everything in the manifest, then repeatedly imports the
// dependencies declared by installed addons and installs them, until a round adds
// nothing new (or the round cap is hit). Dependency assets are resolved over the
// network automatically; each step is reported.
func InstallAllDeps(ctx context.Context, manifestPath, baseDir string, report Reporter) ([]InstallOutcome, error) {
	var outcomes []InstallOutcome
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

		added := importDeps(ctx, manifestPath, baseDir, report)
		if added == 0 {
			return outcomes, nil
		}
		report("Added %d dependenc%s to the manifest; installing…", added, plural(added))
	}
	report("Dependency resolution stopped after %d rounds.", maxDepRounds)
	return outcomes, nil
}

// importDeps scans every installed addon for declared dependencies the manifest does
// not yet satisfy, resolves each (tagless → repo-only, tagged → release asset), and
// appends it to the manifest. It writes nothing it can't resolve. It returns the
// number of entries added so the caller knows whether another install round is due.
func importDeps(parent context.Context, manifestPath, baseDir string, report Reporter) int {
	statuses, err := Inspect(manifestPath, baseDir)
	if err != nil {
		return 0
	}
	manifest, err := Parse(manifestPath)
	if err != nil {
		return 0
	}

	// Dedup across addons by repo identity so a dep declared by two installed addons
	// is resolved once.
	missing := make(map[string]Dependency)
	for _, s := range statuses {
		if !s.Present() {
			continue
		}
		deps, err := MissingDeps(s.Addon, baseDir, manifest)
		if err != nil {
			continue
		}
		for _, d := range deps {
			if _, seen := missing[d.RepoID]; !seen {
				missing[d.RepoID] = d
			}
		}
	}
	if len(missing) == 0 {
		return 0
	}

	ctx, cancel := context.WithTimeout(parent, DepsResolveTimeout)
	defer cancel()

	added := 0
	for _, d := range missing {
		name, ok, err := AddDepEntry(ctx, manifestPath, d, false)
		if err != nil {
			report("  -> Could not add %s: %v", d.RepoID, err)
			continue
		}
		if !ok {
			report("  -> Skipping %s: no asset for %s", d.RepoID, d.Tag)
			continue
		}
		if d.Tag == "" {
			report("  -> Added %s (no version)", name)
		} else {
			report("  -> Added %s %s", name, d.Tag)
		}
		added++
	}
	return added
}

// AddDepEntry resolves one declared dependency and appends it to the manifest: a
// tagless dep is added repo-only (Install clones it; the user can pin a tag later),
// a tagged dep is added at its resolved release asset (archive-first, then network —
// see ResolveDepAsset). asDependency records the is_dependency provenance on the new
// entry. added is false (with err nil) when a tagged dep's asset can't be resolved;
// the caller decides how to report the skip. The ctx only bounds the asset lookup —
// the caller owns the timeout.
func AddDepEntry(ctx context.Context, manifestPath string, d Dependency, asDependency bool) (name string, added bool, err error) {
	name = DeriveName(d.RepoURL)
	entry := Addon{Name: name, URL: NormalizeRepoURL(d.RepoURL), Dependency: asDependency}
	if d.Tag != "" {
		asset, ok := ResolveDepAsset(ctx, d)
		if !ok {
			return name, false, nil
		}
		entry.URL, entry.Tag = asset.URL, d.Tag
	}
	// AddEntryFull with an empty tag behaves like a bare AddEntry; unlike AddEntry it
	// also records the is_dependency provenance when asDependency is set.
	if err := AddEntryFull(manifestPath, entry); err != nil {
		return name, false, err
	}
	return name, true, nil
}

// ResolveDepAsset finds the dependency's required release and picks its install asset
// (source.AutoAsset: the single uploaded build, or the generated source archive
// when none was uploaded; ambiguous multi-upload releases yield ok=false).
//
// It is archive-first: a tag-equal local copy avoids the network and survives upstream
// delisting. It falls through to the network when the archive has no (unambiguous) match.
func ResolveDepAsset(ctx context.Context, d Dependency) (source.Asset, bool) {
	if asset, ok := archivedDepAsset(d); ok {
		return asset, true
	}
	listing, err := source.AvailableVersions(ctx, d.RepoURL)
	if err != nil || listing == nil {
		return source.Asset{}, false
	}
	for _, rel := range listing.Releases {
		if tagEqual(rel.Tag, d.Tag) {
			return source.AutoAsset(rel)
		}
	}
	return source.Asset{}, false
}

// archivedDepAsset returns a locally archived asset for the dependency's required tag,
// if one exists. The archive is keyed by tag, so this only applies to tagged deps; an
// ambiguous archive (multiple stored assets at the tag) yields ok=false and lets the
// caller fall through to the network.
func archivedDepAsset(d Dependency) (source.Asset, bool) {
	releases, err := archive.List(d.RepoID)
	if err != nil {
		return source.Asset{}, false
	}
	for _, rel := range releases {
		if tagEqual(rel.Tag, d.Tag) {
			return source.AutoAsset(rel)
		}
	}
	return source.Asset{}, false
}

// tagEqual matches a required tag against a release tag, tolerating a leading "v" on
// either side (e.g. "1.2.0" matches "v1.2.0").
func tagEqual(a, b string) bool {
	return a == b || strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

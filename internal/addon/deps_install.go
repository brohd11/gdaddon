package addon

import (
	"context"
	"time"

	"github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/source"

	"github.com/brohd11/goutil/strutil"
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
//
// confirm, when non-nil, vets each *newly discovered* dependency before it is recorded.
// Entries the manifest already lists — including is_dependency ones a previous run
// added — are installed by InstallAll without asking: they are already in the user's
// own file, and re-confirming them every run would make the manifest meaningless.
// A confirmer error (ErrDepAborted) stops the loop and is returned with whatever was
// installed so far.
func InstallAllDeps(ctx context.Context, manifestPath, baseDir string, confirm DepConfirmer, report Reporter) ([]InstallOutcome, error) {
	var outcomes []InstallOutcome
	// Declining is remembered across rounds: importDeps recomputes the missing set from
	// scratch each time, so without this a declined dependency would be re-offered on
	// every round until the cap.
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

// importDeps scans every installed addon for declared dependencies the manifest does
// not yet satisfy, resolves each (tagless → repo-only, tagged → release asset), and
// appends it to the manifest. It writes nothing it can't resolve. It returns the
// number of entries added so the caller knows whether another install round is due.
//
// confirm vets each dependency after it resolves and before it is written, so a
// decline adds nothing; declined RepoIDs are recorded in declined (owned by the caller
// across rounds) so the same one is never offered twice. A confirmer error aborts.
func importDeps(parent context.Context, manifestPath, baseDir string, confirm DepConfirmer, declined map[string]bool, report Reporter) (int, error) {
	statuses, err := Inspect(manifestPath, baseDir)
	if err != nil {
		return 0, nil
	}
	manifest, err := Parse(manifestPath)
	if err != nil {
		return 0, nil
	}

	// Dedup across addons by repo identity so a dep declared by two installed addons is
	// resolved once, keeping declaration order — with a confirmer attached the order
	// these are offered in is user-visible, so it can't come from map iteration.
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

// AddDepEntry resolves one declared dependency and appends it to the manifest: a
// clone dep is added as a checkout of its branch, a tagless dep is added repo-only
// (Install clones it; the user can pin a tag later), and a tagged dep is added at its
// resolved release asset (archive-first, then network — see ResolveDepAsset).
// asDependency records the is_dependency provenance on the new entry. added is false
// (with err nil) when a tagged dep's asset can't be resolved; the caller decides how to
// report the skip. The ctx only bounds the asset lookup — the caller owns the timeout.
func AddDepEntry(ctx context.Context, manifestPath string, d Dependency, asDependency bool) (entry Addon, added bool, err error) {
	asset, ok := source.Asset{}, true
	if needsDepAsset(d) {
		d, asset, ok = ResolveDepAsset(ctx, d)
	}
	return writeDepEntry(manifestPath, d, asset, ok, asDependency)
}

// needsDepAsset reports whether recording d requires resolving a release asset first.
// A clone checks out a branch and a tagless dep is recorded repo-only, so neither has a
// release to look up — and neither should pay for (or fail on) a network call.
func needsDepAsset(d Dependency) bool { return !d.IsClone() && d.Tag != "" }

// DepLabel renders what an entry recorded from a dependency actually is, for progress
// lines and confirm prompts. A clone is called out as such rather than shown as a bare
// ref: it brings a .git and tracks a branch, which is a materially different thing to
// agree to than a pinned snapshot. It takes the recorded kind and ref rather than a
// Dependency so the TUI's already-resolved plan rows share the one definition.
//
// The ref must be the resolved one — an `@latest` dep labels the tag it landed on, never
// the word.
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

// writeDepEntry is AddDepEntry's post-resolution half: it records the dependency from
// an already-resolved asset. Split out so a caller that must resolve first — to show a
// DepConfirmer the url it is about to download — can commit that same resolution
// instead of paying for a second lookup. resolved is ResolveDepAsset's ok; false means
// the tag had no asset, which is a skip (added false, err nil), not an error.
//
// It returns the entry it wrote, not just the name, so a caller reports what actually
// landed in the manifest — a clone's kind, or the tag an `@latest` spec resolved to —
// rather than re-deriving a label from the spec and getting to disagree with the file.
// The entry is filled in even when added is false, so the name is always usable.
func writeDepEntry(manifestPath string, d Dependency, asset source.Asset, resolved, asDependency bool) (entry Addon, added bool, err error) {
	name := EntryKey(d.RepoURL)
	if d.IsClone() {
		entry = CloneEntry(d, name, asDependency)
		if err := AddEntryFull(manifestPath, entry); err != nil {
			return entry, false, err
		}
		return entry, true, nil
	}
	entry = Addon{Name: name, URL: NormalizeRepoURL(d.RepoURL), Dependency: asDependency}
	if d.Tag != "" {
		if !resolved {
			return entry, false, nil
		}
		entry.URL, entry.Tag = asset.URL, d.Tag
	}
	// AddEntryFull with an empty tag behaves like a bare AddEntry; unlike AddEntry it
	// also records the is_dependency provenance when asDependency is set.
	if err := AddEntryFull(manifestPath, entry); err != nil {
		return entry, false, err
	}
	return entry, true, nil
}

// CloneEntry builds the manifest entry a clone dependency is recorded as: the canonical
// .git url, the branch in Tag (empty meaning whatever the remote's default is, which the
// install records afterwards), and kind: clone, which AddEntryFull writes out.
//
// The CLI's `gdaddon install clone:owner/repo` builds its entry from here too, for the
// same reason it shares ParseRepoSpec: a hand-typed spec and a declared dependency must
// not be able to disagree about what a clone is.
func CloneEntry(d Dependency, name string, asDependency bool) Addon {
	return Addon{
		Name:       name,
		URL:        NormalizeRepoURL(d.RepoURL),
		Tag:        d.Tag,
		Kind:       KindClone,
		Dependency: asDependency,
	}
}

// ResolveDepAsset finds the dependency's required release or Git tag and picks its install asset
// (source.AutoAsset: the single uploaded build, or the generated source archive
// when none was uploaded or no release exists; ambiguous multi-upload releases yield ok=false).
//
// It is archive-first: a tag-equal local copy avoids the network and survives upstream
// delisting. It falls through to the network when the archive has no (unambiguous) match.
//
// It returns the dependency back because an `@latest` spec names no particular version:
// the word is resolved here, once, and the *returned* d carries the concrete tag that
// callers must record. Writing d.Tag straight to the manifest would pin the word instead
// of the release, which is not a pin at all.
func ResolveDepAsset(ctx context.Context, d Dependency) (Dependency, source.Asset, bool) {
	// The archive is keyed by tag, so there is nothing to look up until `latest` has a
	// value — and a repo that really does ship a rolling `latest` tag must not have a
	// stale archived copy of it shadow the newest release.
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

// TagEqual is the exported wrapper over tagEqual, for callers outside the package
// matching a requested tag against a release tag.
func TagEqual(a, b string) bool { return tagEqual(a, b) }

// tagEqual matches a required tag against a release tag, tolerating a leading "v" on
// either side (e.g. "1.2.0" matches "v1.2.0").
func tagEqual(a, b string) bool {
	return source.TagEqual(a, b)
}

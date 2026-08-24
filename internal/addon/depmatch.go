package addon

import (
	"path/filepath"

	"github.com/brohd11/gdaddon/internal/source"
)

// This file holds the one rule the whole dependency system turns on: given a dependency
// a plugin.cfg declares, which manifest entry (if any) is it, and does that entry satisfy
// it? Every consumer — the missing-deps warning, `list --json`, the Dependencies screen,
// the recursive installer, and the TUI's "Get deps" flow — answers those two questions
// through depIndex and depSatisfied rather than re-deriving them, because they used to
// each carry their own copy and one of the copies was wrong (see depIndex).

// depIndex matches declared dependencies against recorded manifest entries.
//
// It is the single implementation of the **upstream-rename fallback**: a dep is matched by
// canonical repo identity first, then by the name it would be recorded under. That second
// step is load-bearing and easy to forget. A repo renamed upstream keeps serving its
// release assets under the *new* name, so the manifest records an id that the declared
// `deps` spec — still naming the old one — no longer parses to. Without the fallback such
// a dep reads as perpetually missing: the TUI nags forever, `list --json` reports it in
// missing_deps, and "Add all" / `install --all` fail on it with
// `already added from <new-id> (as "<name>")`. brohd11/Godot-TreeSitter-Wrapper →
// godot-tree-sitter-gd is a live instance of this in the wild.
//
// Positions are stored rather than entries so a caller holding a richer slice ([]Status)
// can index back into its own: build the index from the entries, use find's result against
// the original.
type depIndex struct {
	byRepo map[string]int
	byName map[string]int
}

// newDepIndex indexes entries by canonical repo id and by name. Entries whose url doesn't
// parse have no repo identity and are reachable by name alone; later duplicates win, which
// matches the map-building the four call sites did before this existed.
func newDepIndex(entries []Addon) depIndex {
	ix := depIndex{
		byRepo: make(map[string]int, len(entries)),
		byName: make(map[string]int, len(entries)),
	}
	for i, e := range entries {
		if id, err := source.RepoID(e.URL); err == nil {
			ix.byRepo[id] = i
		}
		ix.byName[e.Name] = i
	}
	return ix
}

// find returns the position of the entry d refers to, or -1 when the manifest has none.
func (ix depIndex) find(d Dependency) int {
	if i, ok := ix.byRepo[d.RepoID]; ok {
		return i
	}
	if i, ok := ix.byName[DeriveName(d.RepoURL)]; ok {
		return i
	}
	return -1
}

// addonsOf projects the inspected statuses onto their manifest entries, so []Status
// callers can build a depIndex over the same identities []Addon callers do.
func addonsOf(statuses []Status) []Addon {
	entries := make([]Addon, len(statuses))
	for i, s := range statuses {
		entries[i] = s.Addon
	}
	return entries
}

// declaredDeps reads what a declares from its installed plugin.cfg. An entry with no
// recorded path isn't installed, so there is no config to read and it declares nothing —
// the guard every dependency reader needs before touching the disk.
func declaredDeps(a Addon, projectRoot string) ([]Dependency, error) {
	if a.Path == "" {
		return nil, nil
	}
	return Dependencies(filepath.Join(projectRoot, a.Path))
}

// depSatisfied reports whether an entry recorded at recordedTag meets d. A tagless dep is
// satisfied by presence alone, and a tag pair that cannot be compared (a date stamp, a
// branch checkout with no tag) is *trusted* rather than treated as a miss — deliberate
// HEAD-tracking is not something to nag about.
func depSatisfied(d Dependency, recordedTag string) bool {
	if d.Tag == "" {
		return true
	}
	sat, verified := d.SatisfiedByTag(recordedTag)
	return !verified || sat
}

// StaleDep is a dependency whose manifest entry exists but is verifiably older than the
// declared requirement. Recorded is the tag the manifest has, for the message a caller
// shows ("has v1.0.0, needs v2.0.0"); it is empty when the entry records no tag.
type StaleDep struct {
	Dep      Dependency
	Recorded string
}

// DepPlan is the three-way classification of one addon's declared dependencies against the
// manifest: already satisfied, absent and addable, or present but verifiably stale. It is
// manifest-presence only — nothing here reads the disk or the network, so it is cheap
// enough to recompute on a refresh, and a caller that needs install state wants DepStatuses
// instead.
type DepPlan struct {
	Add       []Dependency // no manifest entry: what "Add all missing" would record
	Satisfied []Dependency // present at a sufficient (or unverifiable) tag
	Stale     []StaleDep   // present but older than required
}

// PlanDeps classifies every dependency a declares (from its installed plugin.cfg under
// projectRoot) against the manifest. Suppressed deps (a.SuppressDeps) are omitted from all
// three buckets — the user has said they don't want to hear about them. A not-installed
// addon (no path, or no plugin.cfg on disk) declares nothing and yields a zero plan.
//
// This is the shared classification behind MissingDeps and the TUI's dependency flows; the
// asset resolution needed to actually *add* an entry is deliberately not here, because it
// is a network call and this stays local.
func PlanDeps(a Addon, projectRoot string, manifest []Addon) (DepPlan, error) {
	var plan DepPlan
	deps, err := declaredDeps(a, projectRoot)
	if err != nil || len(deps) == 0 {
		return plan, err
	}

	ix := newDepIndex(manifest)
	suppressed := stringSet(a.SuppressDeps)

	for _, d := range deps {
		if suppressed[d.RepoID] {
			continue
		}
		i := ix.find(d)
		switch {
		case i < 0:
			plan.Add = append(plan.Add, d)
		case depSatisfied(d, manifest[i].Tag):
			plan.Satisfied = append(plan.Satisfied, d)
		default:
			plan.Stale = append(plan.Stale, StaleDep{Dep: d, Recorded: manifest[i].Tag})
		}
	}
	return plan, nil
}

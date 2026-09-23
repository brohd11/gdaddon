package addon

import (
	"path/filepath"

	"github.com/brohd11/gdaddon/internal/source"
)

// The rule every dependency consumer shares: which manifest entry a declared dependency is,
// and whether it satisfies it (depIndex and depSatisfied).

// depIndex matches declared dependencies against manifest entries, by canonical repo
// identity first, then by name. The name fallback handles upstream renames: the manifest
// records the new repo id while plugin.cfg specs still name the old one, which would
// otherwise read as permanently missing. Positions are stored so callers can index their
// own richer slices.
type depIndex struct {
	byRepo map[string]int
	byName map[string]int
}

// newDepIndex indexes entries by repo id and by every name they answer to (the key's slug
// and the declared name), since specs name repos, not identity keys. Unparseable urls are
// reachable by name only; later duplicates win.
func newDepIndex(entries []Addon) depIndex {
	ix := depIndex{
		byRepo: make(map[string]int, len(entries)),
		byName: make(map[string]int, len(entries)),
	}
	for i, e := range entries {
		if id, err := source.RepoID(e.URL); err == nil {
			ix.byRepo[id] = i
		}
		ix.byName[e.Slug()] = i
		if e.Display != "" {
			ix.byName[e.Display] = i
		}
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

// declaredDeps reads a's installed plugin.cfg dependencies; an entry with no path declares
// nothing.
func declaredDeps(a Addon, projectRoot string) ([]Dependency, error) {
	if a.Path == "" {
		return nil, nil
	}
	return Dependencies(filepath.Join(projectRoot, a.Path))
}

// depSatisfied reports whether an entry at recordedTag meets d. Tagless deps are satisfied
// by presence, and uncomparable tags are trusted rather than nagged about. Clone deps are
// satisfied by presence of any kind; checked separately because a branch may look like a
// version.
func depSatisfied(d Dependency, recordedTag string) bool {
	if d.Tag == "" || d.IsClone() {
		return true
	}
	sat, verified := d.SatisfiedByTag(recordedTag)
	return !verified || sat
}

// StaleDep is a dependency whose entry is verifiably older than required; Recorded is the
// entry's tag (maybe empty).
type StaleDep struct {
	Dep      Dependency
	Recorded string
}

// DepPlan classifies an addon's dependencies against the manifest: satisfied, missing, or
// stale. Manifest only, no disk or network, so it is cheap to recompute.
type DepPlan struct {
	Add       []Dependency // no manifest entry: what "Add all missing" would record
	Satisfied []Dependency // present at a sufficient (or unverifiable) tag
	Stale     []StaleDep   // present but older than required
}

// PlanDeps classifies a's declared dependencies (from its installed plugin.cfg) against
// the manifest, omitting suppressed ones. An uninstalled addon yields a zero plan. Asset
// resolution (network) is not done here.
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

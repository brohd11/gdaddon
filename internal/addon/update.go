package addon

import (
	"context"
	"sort"
	"sync"

	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/version"
)

// UpdateState describes whether a newer release than the installed one is
// available for an addon, as far as the repo's release listing can tell.
type UpdateState int

const (
	UpdateUnknown   UpdateState = iota // not checked, branch-tracked, no releases, or unresolvable url
	UpdateCurrent                      // the installed release is current or semantically newer
	UpdateAvailable                    // a newer release than the pinned one exists
	UpdateLocked                       // the entry is locked: updates are intentionally not checked
)

// String renders an UpdateState as a short lowercase label for non-interactive
// output, mirroring State.String().
func (s UpdateState) String() string {
	switch s {
	case UpdateCurrent:
		return "current"
	case UpdateAvailable:
		return "available"
	case UpdateLocked:
		return "locked"
	default:
		return "unknown"
	}
}

// UpdateInfo is the cached result of one addon's update check.
type UpdateInfo struct {
	State     UpdateState
	LatestTag string // the latest release's tag, when known
}

// CheckUpdate reports an update only for a semantically newer release. An exact asset
// match proves the current release; missing or uncomparable versions report UpdateUnknown.
func CheckUpdate(ctx context.Context, a Addon) UpdateInfo {
	if a.URL == "" {
		return UpdateInfo{}
	}
	// Live git checkouts (clone/submodule) are updated via git directly; they track a
	// branch, not a release tag, so there's nothing to flag.
	if a.IsGitWorkdir() {
		return UpdateInfo{}
	}
	// A commit-pinned package is a deliberate snapshot with nothing to compare against.
	if a.Commit != "" {
		return UpdateInfo{}
	}
	// Locked entries skip the network and report UpdateLocked (distinct from Unknown).
	if a.IsLocked() {
		return UpdateInfo{State: UpdateLocked}
	}
	latest, state := walkUpdate(ctx, a)
	if state == UpdateUnknown {
		return UpdateInfo{}
	}
	return UpdateInfo{State: state, LatestTag: latest.Tag}
}

// walkUpdate fetches the releases and classifies the pinned url against the latest,
// returning that release. Anything undecidable (fetch error, no releases, branch url,
// uncomparable versions) is UpdateUnknown.
func walkUpdate(ctx context.Context, a Addon) (source.Release, UpdateState) {
	// These exclusions apply equally to prompts and bulk update plans.
	if a.IsGitWorkdir() || a.Commit != "" || a.IsLocked() {
		return source.Release{}, UpdateUnknown
	}
	listing, err := source.AvailableVersions(ctx, a.URL)
	if err != nil || listing == nil {
		return source.Release{}, UpdateUnknown
	}
	// A branch-tracked install follows HEAD, which can't be matched against a
	// release tag — leave it unknown rather than always flagging an update.
	if listing.Branch != nil {
		for _, asset := range listing.Branch.Assets {
			if asset.URL == a.URL {
				return source.Release{}, UpdateUnknown
			}
		}
	}

	latest, ok := LatestRelease(listing.Releases)
	if !ok {
		return source.Release{}, UpdateUnknown
	}
	// On the latest release: its url is one of that release's assets.
	for _, asset := range latest.Assets {
		if asset.URL == a.URL {
			return latest, UpdateCurrent
		}
	}
	// A matched asset identifies the installed release, but does not prove it is
	// older. Prefer that identity over potentially stale manifest metadata.
	if installed, ok := releaseForURL(a.URL, listing.Releases); ok {
		a.Tag = installed.Tag
		a.Version = ""
	}
	if current, ok := currentByVersion(a, latest.Tag); ok {
		if current {
			return latest, UpdateCurrent
		}
		return latest, UpdateAvailable
	}
	return source.Release{}, UpdateUnknown
}

// releaseForURL identifies the release owning a pinned asset URL.
func releaseForURL(url string, releases []source.Release) (source.Release, bool) {
	for _, rel := range releases {
		for _, asset := range rel.Assets {
			if asset.URL == url {
				return rel, true
			}
		}
	}
	return source.Release{}, false
}

// currentByVersion compares the installed tag (or config version) with latestTag by
// semver; ok is false when neither is comparable.
func currentByVersion(a Addon, latestTag string) (current, ok bool) {
	installed := a.Tag
	if installed == "" {
		installed = a.Version
	}
	if installed == "" {
		return false, false
	}
	return semverGE(installed, latestTag)
}

// UpdatePlan is one update to perform: the addon, its current version, and the latest tag
// and asset. Produced by ResolveUpdate, consumed by UpdateAll.
type UpdatePlan struct {
	Addon      Addon
	OldVersion string
	NewTag     string
	Asset      source.Asset
}

// UpdateResolution is the outcome of resolving one addon's update.
type UpdateResolution int

const (
	ResolveNone      UpdateResolution = iota // nothing to do (current / branch / locked / uncomparable / no releases)
	ResolvePlan                              // a plan is available (UpdatePlan valid)
	ResolveAmbiguous                         // a newer release exists but several uploaded packages make the asset choice ambiguous
)

// ResolveUpdate returns a plan when a newer release exists, choosing its asset with
// source.AutoAsset. ResolveNone covers up to date, branch-tracked, locked, uncomparable and
// unfetchable; ResolveAmbiguous a newer release with several uploads.
func ResolveUpdate(ctx context.Context, a Addon, localVersion string) (UpdatePlan, UpdateResolution) {
	if a.URL == "" {
		return UpdatePlan{}, ResolveNone
	}
	// A locked entry is pinned by the user: never plan a bulk update for it.
	if a.IsLocked() {
		return UpdatePlan{}, ResolveNone
	}
	latest, state := walkUpdate(ctx, a)
	if state != UpdateAvailable {
		return UpdatePlan{}, ResolveNone
	}
	asset, ok := source.AutoAsset(latest)
	if !ok {
		// ok=false is either an empty release (nothing to do) or 2+ uploaded packages
		// (ambiguous — no user to pick, so surface it as a skip).
		if uploadedCount(latest) >= 2 {
			return UpdatePlan{Addon: a, OldVersion: localVersion, NewTag: latest.Tag}, ResolveAmbiguous
		}
		return UpdatePlan{}, ResolveNone
	}
	return UpdatePlan{Addon: a, OldVersion: localVersion, NewTag: latest.Tag, Asset: asset}, ResolvePlan
}

// uploadedCount counts a release's uploaded assets (not the generated archive), to tell
// ambiguous from empty.
func uploadedCount(rel source.Release) int {
	n := 0
	for _, a := range rel.Assets {
		if !a.Generated {
			n++
		}
	}
	return n
}

// maxConcurrentChecks caps concurrent release fetches (host rate limits).
const maxConcurrentChecks = 8

// forInstalled runs fn concurrently (capped) over installed, url-bearing addons and
// collects the kept results, in no particular order. fn honors ctx.
func forInstalled[T any](statuses []Status, fn func(a Addon, local string) (T, bool)) []T {
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentChecks)
	var out []T
	for _, s := range statuses {
		if !s.Present() || s.Addon.URL == "" {
			continue
		}
		wg.Add(1)
		go func(a Addon, local string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if v, ok := fn(a, local); ok {
				mu.Lock()
				out = append(out, v)
				mu.Unlock()
			}
		}(s.Addon, s.LocalVersion)
	}
	wg.Wait()
	return out
}

// CheckUpdates returns each installed addon's CheckUpdate result by name; ctx bounds the
// batch.
func CheckUpdates(ctx context.Context, statuses []Status) map[string]UpdateInfo {
	type nameInfo struct {
		name string
		info UpdateInfo
	}
	results := forInstalled(statuses, func(a Addon, _ string) (nameInfo, bool) {
		return nameInfo{a.Name, CheckUpdate(ctx, a)}, true
	})
	checks := make(map[string]UpdateInfo, len(results))
	for _, r := range results {
		checks[r.name] = r.info
	}
	return checks
}

// SkippedUpdate is an addon with a newer release whose asset can't be chosen
// automatically (several uploaded packages) — surfaced so the user updates it by hand.
type SkippedUpdate struct {
	Name string // the addon's label, for reporting; this carries no manifest identity
	Tag  string
}

// ResolveUpdatePlans returns update plans for installed addons with newer releases, plus
// those whose newer release has an ambiguous asset. Fetches run concurrently under ctx;
// both lists are sorted by name.
func ResolveUpdatePlans(ctx context.Context, manifestPath, baseDir string) ([]UpdatePlan, []SkippedUpdate, error) {
	statuses, err := Inspect(manifestPath, baseDir)
	if err != nil {
		return nil, nil, err
	}
	type result struct {
		plan    UpdatePlan
		skipped SkippedUpdate
		isSkip  bool
	}
	results := forInstalled(statuses, func(a Addon, local string) (result, bool) {
		plan, res := ResolveUpdate(ctx, a, local)
		switch res {
		case ResolvePlan:
			return result{plan: plan}, true
		case ResolveAmbiguous:
			return result{skipped: SkippedUpdate{Name: a.Label(), Tag: plan.NewTag}, isSkip: true}, true
		default:
			return result{}, false
		}
	})
	var plans []UpdatePlan
	var skipped []SkippedUpdate
	for _, r := range results {
		if r.isSkip {
			skipped = append(skipped, r.skipped)
		} else {
			plans = append(plans, r.plan)
		}
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Addon.Name < plans[j].Addon.Name })
	sort.Slice(skipped, func(i, j int) bool { return skipped[i].Name < skipped[j].Name })
	return plans, skipped, nil
}

// UpdateAll installs each plan and pins the result, reporting progress; one failure does
// not stop the rest.
func UpdateAll(ctx context.Context, manifestPath string, plans []UpdatePlan, baseDir string, report Reporter) ([]InstallOutcome, error) {
	var outcomes []InstallOutcome
	for _, p := range plans {
		a := p.Addon
		old := p.OldVersion
		if old == "" {
			old = "Unknown/None"
		}
		report("[%s] Updating %s → %s...", a.Label(), old, p.NewTag)

		target := Addon{Name: a.Name, URL: p.Asset.URL, Path: a.Path, Tag: p.NewTag}
		res, err := Install(ctx, target, baseDir, report)
		if err != nil {
			report("[%s] Error: %v", a.Label(), err)
			continue
		}
		if res.Path != "" {
			version := res.Version
			if version == "" {
				version = TagVersion(p.NewTag)
			}
			if err := UpdateEntry(manifestPath, a.Name, p.Asset.URL, res.Path, version, p.NewTag); err != nil {
				report("[%s] Error pinning manifest: %v", a.Label(), err)
				continue
			}
			if err := AdoptName(manifestPath, a, res); err != nil {
				report("[%s] Could not record the declared name: %v", a.Label(), err)
			}
			outcomes = append(outcomes, InstallOutcome{
				Name: a.Name, Display: displayOf(a, res), URL: a.URL,
				PriorPath: a.Path, Path: res.Path, Version: version,
			})
		}
	}
	return outcomes, nil
}

// LatestRelease picks the highest semantic stable release, else prereleases. Uncomparable
// tags count only without semantic ones; ties keep input order.
func LatestRelease(releases []source.Release) (source.Release, bool) {
	var best source.Release
	found := false
	for _, r := range releases {
		if !found || (best.IsPrerelease() && !r.IsPrerelease()) {
			best, found = r, true
			continue
		}
		if best.IsPrerelease() != r.IsPrerelease() {
			continue
		}
		if order, ok := version.Compare(r.Tag, best.Tag); (ok && order > 0) ||
			(!version.IsValid(best.Tag) && version.IsValid(r.Tag)) {
			best = r
		}
	}
	return best, found
}

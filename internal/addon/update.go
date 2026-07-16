package addon

import (
	"context"
	"sort"
	"strings"
	"sync"

	"gdaddon/internal/source"
)

// UpdateState describes whether a newer release than the installed one is
// available for an addon, as far as the repo's release listing can tell.
type UpdateState int

const (
	UpdateUnknown   UpdateState = iota // not checked, branch-tracked, no releases, or unresolvable url
	UpdateCurrent                      // the pinned url is part of the latest release
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

// CheckUpdate fetches the addon's release listing and reports whether its pinned
// url is part of the latest release. "Part of the latest release" means the url
// matches one of that release's assets (an uploaded .zip or the host's generated
// source archive), so release-download and archive urls both compare correctly:
// the same installed version re-resolves to the same asset url, a newer one does
// not. A url-less entry, a branch-tracked url (HEAD has no release tag to compare
// against), a fetch error, or a repo with no releases all read as UpdateUnknown
// so no false notification is shown.
func CheckUpdate(ctx context.Context, a Addon) UpdateInfo {
	if a.URL == "" {
		return UpdateInfo{}
	}
	// Live git checkouts (clone/submodule) are updated via git directly; they track a
	// branch, not a release tag, so there's nothing to flag.
	if a.IsGitWorkdir() {
		return UpdateInfo{}
	}
	// A commit-pinned package is a frozen snapshot chosen deliberately; a sha has no
	// semver "latest" to compare, and its recorded plugin.cfg version must not nag
	// against newer releases. Re-pinning is just re-installing the branch.
	if a.Commit != "" {
		return UpdateInfo{}
	}
	// A locked entry is pinned by the user: don't check (or flag) updates for it. Short
	// circuit before the network fetch and report UpdateLocked — no marker, but
	// distinguishable from an unresolvable UpdateUnknown.
	if a.IsLocked() {
		return UpdateInfo{State: UpdateLocked}
	}
	listing, err := source.AvailableVersions(ctx, a.URL)
	if err != nil || listing == nil {
		return UpdateInfo{}
	}
	// A branch-tracked install follows HEAD, which can't be matched against a
	// release tag — leave it unknown rather than always flagging an update.
	if listing.Branch != nil {
		for _, asset := range listing.Branch.Assets {
			if asset.URL == a.URL {
				return UpdateInfo{}
			}
		}
	}

	latest, ok := LatestRelease(listing.Releases)
	if !ok {
		return UpdateInfo{}
	}
	// On the latest release: its url is one of that release's assets.
	for _, asset := range latest.Assets {
		if asset.URL == a.URL {
			return UpdateInfo{State: UpdateCurrent, LatestTag: latest.Tag}
		}
	}
	// A precisely pinned asset from an older release: definitely outdated.
	if urlInReleases(a.URL, listing.Releases) {
		return UpdateInfo{State: UpdateAvailable, LatestTag: latest.Tag}
	}
	// Otherwise the url is a bare repo/clone url (e.g. a scanned install tracked
	// from a `source=` key) that matches no asset — fall back to comparing the
	// installed version/tag against the latest release tag. Uncomparable versions
	// stay unknown so no false update is flagged.
	if current, ok := currentByVersion(a, latest.Tag); ok {
		if current {
			return UpdateInfo{State: UpdateCurrent, LatestTag: latest.Tag}
		}
		return UpdateInfo{State: UpdateAvailable, LatestTag: latest.Tag}
	}
	return UpdateInfo{}
}

// urlInReleases reports whether url is one of the assets across any of the releases.
func urlInReleases(url string, releases []source.Release) bool {
	for _, rel := range releases {
		for _, asset := range rel.Assets {
			if asset.URL == url {
				return true
			}
		}
	}
	return false
}

// currentByVersion compares the addon's installed identifier (prefer its tag, else
// its plugin.cfg version) against latestTag with semver >=. ok is false when neither
// side is a comparable dotted-numeric version (a date stamp, no version, …) so the
// caller can leave the result unknown rather than flag a false update.
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

// UpdatePlan is a resolved instruction to update one addon: the addon as it
// stands in the manifest, the version it's on now, and the latest release's tag
// plus the asset to install for it. Produced by ResolveUpdate, consumed by
// UpdateAll (and rendered in the update-all confirm).
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

// ResolveUpdate fetches the addon's release listing and, when a newer release than the
// installed one exists, returns the plan to install it. The target asset is chosen by
// source.AutoAsset (prefer the uploaded package, else the generated source archive) — the
// same selector as Install latest. It returns ResolveNone when the addon is already on the
// latest release, is branch-tracked, locked, has no comparable releases, or can't be
// fetched (so it never plans a no-op update), and ResolveAmbiguous when a newer release
// exists but has two or more uploaded packages (no user to pick — the caller logs & skips).
func ResolveUpdate(ctx context.Context, a Addon, localVersion string) (UpdatePlan, UpdateResolution) {
	if a.URL == "" {
		return UpdatePlan{}, ResolveNone
	}
	// A locked entry is pinned by the user: never plan a bulk update for it.
	if a.IsLocked() {
		return UpdatePlan{}, ResolveNone
	}
	listing, err := source.AvailableVersions(ctx, a.URL)
	if err != nil || listing == nil {
		return UpdatePlan{}, ResolveNone
	}
	if listing.Branch != nil {
		for _, asset := range listing.Branch.Assets {
			if asset.URL == a.URL {
				return UpdatePlan{}, ResolveNone
			}
		}
	}
	latest, ok := LatestRelease(listing.Releases)
	if !ok {
		return UpdatePlan{}, ResolveNone
	}
	// Already on the latest release: its url is one of that release's assets.
	for _, asset := range latest.Assets {
		if asset.URL == a.URL {
			return UpdatePlan{}, ResolveNone
		}
	}
	// A bare repo/clone url (a scanned install) matches no asset, so fall back to a
	// version/tag comparison: don't plan an update when it's already current or the
	// version can't be compared — only when it's verifiably older.
	if !urlInReleases(a.URL, listing.Releases) {
		current, ok := currentByVersion(a, latest.Tag)
		if !ok || current {
			return UpdatePlan{}, ResolveNone
		}
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

// uploadedCount counts a release's author-uploaded assets (excluding the host's
// generated source archive) — used to tell an ambiguous release (2+ uploads) from an
// empty one when source.AutoAsset returns ok=false.
func uploadedCount(rel source.Release) int {
	n := 0
	for _, a := range rel.Assets {
		if !a.Generated {
			n++
		}
	}
	return n
}

// maxConcurrentChecks caps how many release-listing fetches forInstalled runs at
// once, so a large manifest can't fire hundreds of parallel requests (and trip host
// rate limits). For a typical manifest it's effectively parallel.
const maxConcurrentChecks = 8

// forInstalled runs fn concurrently over every installed, url-bearing addon in
// statuses and collects the results fn keeps (ok == true). Not-installed or url-less
// entries are skipped (nothing to compare). It's the shared fan-out behind
// CheckUpdates and ResolveUpdatePlans; fn honors ctx (cancel/deadline) itself so a
// slow host can't stall the caller, and in-flight fetches are capped at
// maxConcurrentChecks. Result order is unspecified — callers that need determinism sort.
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

// CheckUpdates resolves the update state of every installed, url-bearing addon in
// statuses concurrently, keyed by addon name. Each entry runs the same CheckUpdate a
// single addon uses; ctx bounds the whole batch.
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
	Name string
	Tag  string
}

// ResolveUpdatePlans inspects the manifest and resolves an update plan for every
// installed addon that has a newer release than the one installed. Not-installed
// or url-less entries are skipped (nothing to compare). Fetches run concurrently
// and honor ctx (cancel/deadline) so a slow host can't stall the caller. It also
// returns the addons that have a newer release but an ambiguous asset choice (2+
// uploaded packages), so the caller can report them for a manual update. Both slices
// are name-sorted for deterministic output.
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
			return result{skipped: SkippedUpdate{Name: a.Name, Tag: plan.NewTag}, isSkip: true}, true
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

// UpdateAll installs each plan's target asset under baseDir and pins the new
// url/path/version back into the manifest, reporting progress per addon. Plans
// come from ResolveUpdate; an empty slice is a no-op. A single addon's failure is
// reported and skipped so the rest still update.
func UpdateAll(ctx context.Context, manifestPath string, plans []UpdatePlan, baseDir string, report Reporter) ([]InstallOutcome, error) {
	var outcomes []InstallOutcome
	for _, p := range plans {
		a := p.Addon
		old := p.OldVersion
		if old == "" {
			old = "Unknown/None"
		}
		report("[%s] Updating %s → %s...", a.Name, old, p.NewTag)

		target := Addon{Name: a.Name, URL: p.Asset.URL, Path: a.Path, Tag: p.NewTag}
		res, err := Install(ctx, target, baseDir, report)
		if err != nil {
			report("[%s] Error: %v", a.Name, err)
			continue
		}
		if res.Path != "" {
			version := res.Version
			if version == "" {
				version = strings.TrimPrefix(p.NewTag, "v")
			}
			_ = UpdateEntry(manifestPath, a.Name, p.Asset.URL, res.Path, version, p.NewTag)
			outcomes = append(outcomes, InstallOutcome{
				Name: a.Name, URL: a.URL, PriorPath: a.Path, Path: res.Path, Version: version,
			})
		}
	}
	return outcomes, nil
}

// LatestRelease picks the newest non-prerelease (releases come newest-first),
// falling back to the newest release when every one is a prerelease. Shared by the
// per-addon update check and selfupdate.
func LatestRelease(releases []source.Release) (source.Release, bool) {
	if len(releases) == 0 {
		return source.Release{}, false
	}
	for _, r := range releases {
		if !r.Prerelease {
			return r, true
		}
	}
	return releases[0], true
}

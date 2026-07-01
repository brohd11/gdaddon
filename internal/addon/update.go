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

	latest, ok := latestRelease(listing.Releases)
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

// ResolveUpdate fetches the addon's release listing and, when a newer release than
// the installed one exists, returns the plan to install it. The target asset
// preserves the kind the addon currently tracks (resolveUpdateAsset). ok is false
// when the addon is already on the latest release, is branch-tracked, has no
// comparable releases, or can't be fetched — so it never plans a no-op update.
func ResolveUpdate(ctx context.Context, a Addon, localVersion string) (UpdatePlan, bool) {
	if a.URL == "" {
		return UpdatePlan{}, false
	}
	// A locked entry is pinned by the user: never plan a bulk update for it.
	if a.IsLocked() {
		return UpdatePlan{}, false
	}
	listing, err := source.AvailableVersions(ctx, a.URL)
	if err != nil || listing == nil {
		return UpdatePlan{}, false
	}
	if listing.Branch != nil {
		for _, asset := range listing.Branch.Assets {
			if asset.URL == a.URL {
				return UpdatePlan{}, false
			}
		}
	}
	latest, ok := latestRelease(listing.Releases)
	if !ok {
		return UpdatePlan{}, false
	}
	// Already on the latest release: its url is one of that release's assets.
	for _, asset := range latest.Assets {
		if asset.URL == a.URL {
			return UpdatePlan{}, false
		}
	}
	// A bare repo/clone url (a scanned install) matches no asset, so fall back to a
	// version/tag comparison: don't plan an update when it's already current or the
	// version can't be compared — only when it's verifiably older.
	if !urlInReleases(a.URL, listing.Releases) {
		current, ok := currentByVersion(a, latest.Tag)
		if !ok || current {
			return UpdatePlan{}, false
		}
	}
	asset, ok := resolveUpdateAsset(a.URL, listing.Releases, latest)
	if !ok {
		return UpdatePlan{}, false
	}
	return UpdatePlan{Addon: a, OldVersion: localVersion, NewTag: latest.Tag, Asset: asset}, true
}

// resolveUpdateAsset picks which asset of the latest release to install for an
// update, preserving the kind the addon currently tracks: it finds the asset name
// the current url resolved to (across any release in the listing) and selects the
// same-named asset in the latest release. When the name can't be matched it falls
// back to the release's last asset — the host's generated source archive, which
// every release offers (see source.resolveReleases) — so an update can still
// proceed.
func resolveUpdateAsset(currentURL string, releases []source.Release, latest source.Release) (source.Asset, bool) {
	var wantName string
	for _, rel := range releases {
		for _, asset := range rel.Assets {
			if asset.URL == currentURL {
				wantName = asset.Name
			}
		}
	}
	if wantName != "" {
		for _, asset := range latest.Assets {
			if asset.Name == wantName {
				return asset, true
			}
		}
	}
	if len(latest.Assets) == 0 {
		return source.Asset{}, false
	}
	return latest.Assets[len(latest.Assets)-1], true
}

// ResolveUpdatePlans inspects the manifest and resolves an update plan for every
// installed addon that has a newer release than the one installed. Not-installed
// or url-less entries are skipped (nothing to compare). Fetches run concurrently
// and honor ctx (cancel/deadline) so a slow host can't stall the caller. The
// returned plans are name-sorted for deterministic output.
func ResolveUpdatePlans(ctx context.Context, manifestPath, baseDir string) ([]UpdatePlan, error) {
	statuses, err := Inspect(manifestPath, baseDir)
	if err != nil {
		return nil, err
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	plans := make([]UpdatePlan, 0)
	for _, s := range statuses {
		if !s.Present() || s.Addon.URL == "" {
			continue
		}
		wg.Add(1)
		go func(a Addon, local string) {
			defer wg.Done()
			if plan, ok := ResolveUpdate(ctx, a, local); ok {
				mu.Lock()
				plans = append(plans, plan)
				mu.Unlock()
			}
		}(s.Addon, s.LocalVersion)
	}
	wg.Wait()
	sort.Slice(plans, func(i, j int) bool { return plans[i].Addon.Name < plans[j].Addon.Name })
	return plans, nil
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

// latestRelease picks the newest non-prerelease (releases come newest-first),
// falling back to the newest release when every one is a prerelease.
func latestRelease(releases []source.Release) (source.Release, bool) {
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

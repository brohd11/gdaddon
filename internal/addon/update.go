package addon

import (
	"context"
	"strings"

	"gdaddon/internal/source"
)

// UpdateState describes whether a newer release than the installed one is
// available for an addon, as far as the repo's release listing can tell.
type UpdateState int

const (
	UpdateUnknown   UpdateState = iota // not checked, branch-tracked, no releases, or unresolvable url
	UpdateCurrent                      // the pinned url is part of the latest release
	UpdateAvailable                    // a newer release than the pinned one exists
)

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
	// Clone entries are live git working copies the user updates via git directly;
	// they track a branch, not a release tag, so there's nothing to flag.
	if a.Clone {
		return UpdateInfo{}
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
	info := UpdateInfo{State: UpdateAvailable, LatestTag: latest.Tag}
	for _, asset := range latest.Assets {
		if asset.URL == a.URL {
			info.State = UpdateCurrent
			break
		}
	}
	return info
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

		target := Addon{Name: a.Name, URL: p.Asset.URL, Path: a.Path}
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

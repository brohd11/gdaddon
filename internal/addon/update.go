package addon

import (
	"context"

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

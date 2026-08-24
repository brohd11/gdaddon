package addon

import (
	"fmt"
	"strings"

	"github.com/brohd11/gdaddon/internal/source"
)

// AmbiguousAssetError reports a release whose uploaded assets can't be picked without
// a user: source.AutoAsset only auto-selects when a release ships exactly one uploaded
// asset (or none, in which case the generated source archive wins). The TUI answers
// this with a picker; the CLI returns this error so the caller can list the candidates
// and point at --asset.
type AmbiguousAssetError struct {
	Tag    string
	Assets []string
}

func (e *AmbiguousAssetError) Error() string {
	return fmt.Sprintf("release %s ships %d assets; pick one with --asset:\n  %s",
		e.Tag, len(e.Assets), strings.Join(e.Assets, "\n  "))
}

// SelectRelease picks the release to install: the one whose tag matches (TagEqual, so
// a leading "v" on either side is tolerated), or — for an empty tag — the latest
// non-prerelease via LatestRelease. A tag that matches nothing yields an error naming
// the tags that are available, so a typo is self-correcting from the message.
func SelectRelease(releases []source.Release, tag string) (source.Release, error) {
	if len(releases) == 0 {
		return source.Release{}, fmt.Errorf("no releases found")
	}
	if tag == "" {
		rel, ok := LatestRelease(releases)
		if !ok {
			return source.Release{}, fmt.Errorf("no releases found")
		}
		return rel, nil
	}
	for _, rel := range releases {
		if tagEqual(rel.Tag, tag) {
			return rel, nil
		}
	}
	return source.Release{}, fmt.Errorf("no release tagged %s; available: %s", tag, tagList(releases))
}

// SelectAsset picks the file to download from a release. With a hint it is the unique
// asset whose name contains it (case-insensitive) — an unmatched or ambiguous hint is
// an error listing the candidates. Without a hint it defers to source.AutoAsset, the
// same selector dependency installs and Update All use, and reports an
// *AmbiguousAssetError when that can't decide.
func SelectAsset(rel source.Release, hint string) (source.Asset, error) {
	if hint == "" {
		if asset, ok := source.AutoAsset(rel); ok {
			return asset, nil
		}
		return source.Asset{}, &AmbiguousAssetError{Tag: rel.Tag, Assets: assetNames(rel.Assets)}
	}

	var matched []source.Asset
	for _, a := range rel.Assets {
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(hint)) {
			matched = append(matched, a)
		}
	}
	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		return source.Asset{}, fmt.Errorf("no asset of %s matches %q; available:\n  %s",
			rel.Tag, hint, strings.Join(assetNames(rel.Assets), "\n  "))
	default:
		return source.Asset{}, &AmbiguousAssetError{Tag: rel.Tag, Assets: assetNames(matched)}
	}
}

// assetNames lists the names of a set of assets, for an error message.
func assetNames(assets []source.Asset) []string {
	names := make([]string, 0, len(assets))
	for _, a := range assets {
		names = append(names, a.Name)
	}
	return names
}

// tagList renders the available release tags for an error message, capped so a repo
// with hundreds of releases doesn't print a wall of text.
func tagList(releases []source.Release) string {
	const max = 10
	tags := make([]string, 0, max)
	for i, rel := range releases {
		if i == max {
			tags = append(tags, fmt.Sprintf("… (%d more)", len(releases)-max))
			break
		}
		tags = append(tags, rel.Tag)
	}
	return strings.Join(tags, ", ")
}

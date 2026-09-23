package addon

import (
	"context"
	"fmt"
	"strings"

	"github.com/brohd11/gdaddon/internal/source"
)

// ResolveVersion keeps untagged installs on published releases; explicit tags may use a
// source package when no release exists. LatestTag resolves like no tag, decided here only.
func ResolveVersion(ctx context.Context, repoURL, tag string) (source.Release, error) {
	if tag != "" && !IsLatestTag(tag) {
		return source.ResolveTag(ctx, repoURL, tag)
	}
	listing, err := source.AvailableVersions(ctx, repoURL)
	if err != nil {
		return source.Release{}, err
	}
	return SelectRelease(listing.Releases, "")
}

// AmbiguousAssetError reports a release with several uploaded assets (AutoAsset picks only
// one or none). The TUI shows a picker; the CLI lists them and points at --asset.
type AmbiguousAssetError struct {
	Tag    string
	Assets []string
}

func (e *AmbiguousAssetError) Error() string {
	return fmt.Sprintf("release %s ships %d assets; pick one with --asset:\n  %s",
		e.Tag, len(e.Assets), strings.Join(e.Assets, "\n  "))
}

// SelectRelease picks the release matching tag (leading "v" tolerated), or the latest
// non-prerelease for "". No match is an error listing the available tags.
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
		if rel.Tag == tag {
			return rel, nil
		}
	}
	for _, rel := range releases {
		if source.TagEqual(rel.Tag, tag) {
			return rel, nil
		}
	}
	return source.Release{}, fmt.Errorf("no release tagged %s; available: %s", tag, tagList(releases))
}

// SelectAsset picks the asset to download: the unique name containing hint
// (case-insensitive), else source.AutoAsset, returning *AmbiguousAssetError when it cannot
// decide.
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

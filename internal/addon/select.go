package addon

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/source"
)

// ResolveVersion finds the release d names: a `/release` tag among published releases only,
// any other tag among git tags (its source package). No tag, or LatestTag, is the newest
// published non-prerelease either way, decided here only.
func ResolveVersion(ctx context.Context, d Dependency) (source.Release, error) {
	if d.Tag != "" && !d.WantsLatest() {
		if !d.Release {
			return source.ResolveSourceTag(ctx, d.RepoURL, d.Tag)
		}
		rel, err := source.ResolveReleaseTag(ctx, d.RepoURL, d.Tag)
		if errors.Is(err, source.ErrNoSuchTag) {
			return rel, fmt.Errorf("%w; drop /release to install the tag's source", err)
		}
		return rel, err
	}
	listing, err := source.AvailableVersions(ctx, d.RepoURL)
	if err != nil {
		return source.Release{}, err
	}
	return SelectRelease(listing.Releases, "")
}

// PickDepAsset picks what d installs from rel without a user: the generated source archive,
// or for `/release` the single upload (the one matching its glob, when it has one).
// Several candidates are an *AmbiguousAssetError; a `/release` never falls back to source.
func PickDepAsset(rel source.Release, d Dependency) (source.Asset, error) {
	if !d.Release {
		if asset, ok := source.GeneratedAsset(rel); ok {
			return asset, nil
		}
		return source.Asset{}, fmt.Errorf("%s has no source package", rel.Tag)
	}
	uploads := source.Uploads(rel)
	if d.AssetPattern != "" {
		var matched []source.Asset
		for _, a := range uploads {
			if ok, _ := path.Match(d.AssetPattern, strings.TrimSuffix(a.Name, archive.ArchivedSuffix)); ok {
				matched = append(matched, a)
			}
		}
		if len(matched) == 0 {
			return source.Asset{}, fmt.Errorf("no asset of %s matches %q; available:\n  %s",
				rel.Tag, d.AssetPattern, strings.Join(assetNames(uploads), "\n  "))
		}
		uploads = matched
	}
	switch len(uploads) {
	case 1:
		return uploads[0], nil
	case 0:
		return source.Asset{}, fmt.Errorf("%s has no uploaded release asset; drop /release to install the tag's source", rel.Tag)
	default:
		return source.Asset{}, &AmbiguousAssetError{Tag: rel.Tag, Assets: assetNames(uploads)}
	}
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

// Package store is the Godot Asset Store (store.godotengine.org) backend: the
// canonical-URL helpers and the releases API client shared by the installer
// (internal/addon), the search backend (internal/search), and the TUI browse flow
// (internal/tui/flows/packages). A store asset is identified in the manifest by its
// canonical URL "https://store.godotengine.org/<publisher>/<slug>"; the actual
// download is resolved from the API at install/browse time. It imports only
// internal/source (for the shared Listing/Release/Asset shapes), internal/restrule
// (the shared authenticated JSON GET), and the stdlib, so it sits below the
// consumers with no cycles.
package store

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"gdaddon/internal/restrule"
	"gdaddon/internal/source"
)

// Host is the Asset Store host. base is the API/page root.
const (
	Host = "store.godotengine.org"
	base = "https://" + Host
)

// IsStoreURL reports whether rawURL is an Asset Store canonical URL (so it installs
// via the store path rather than git/zip). An unparseable url is not a store url.
func IsStoreURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimPrefix(u.Host, "www."), Host)
}

// AssetID extracts the "<publisher>/<slug>" the store API is keyed by from a
// canonical store URL's path (its first two segments).
func AssetID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("not a store asset URL: %q", rawURL)
	}
	return parts[0] + "/" + parts[1], nil
}

// AssetURL is the canonical store URL for an asset id ("<publisher>/<slug>"), the
// stable identity pinned in the manifest.
func AssetURL(id string) string { return base + "/" + id }

// Release is one entry of the store's /api/v1/releases/<id>/ response.
type Release struct {
	Version         string `json:"version"`
	Stable          bool   `json:"stable"`
	MinGodotVersion string `json:"min_godot_version"`
	DownloadURL     string `json:"download_url"`
}

// Releases fetches an asset's releases (newest-first, as the API returns them).
func Releases(ctx context.Context, id string) ([]Release, error) {
	var releases []Release
	if err := restrule.GetJSON(ctx, base+"/api/v1/releases/"+id+"/", &releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// PickStable returns the newest stable release, falling back to the first listed
// when none is flagged stable. ok is false for an empty list.
func PickStable(releases []Release) (Release, bool) {
	if len(releases) == 0 {
		return Release{}, false
	}
	rel := releases[0]
	for _, r := range releases {
		if r.Stable {
			return r, true
		}
	}
	return rel, true
}

// Listing fetches an asset's releases and maps them to the shared source.Listing
// shape so the standard version picker and archive can consume store versions
// unchanged. Each release becomes a tagged release with a single synthesized .zip
// asset pointing at the store download URL.
func Listing(ctx context.Context, rawURL string) (*source.Listing, error) {
	id, err := AssetID(rawURL)
	if err != nil {
		return nil, err
	}
	releases, err := Releases(ctx, id)
	if err != nil {
		return nil, err
	}
	slug := id[strings.IndexByte(id, '/')+1:]

	out := &source.Listing{Owner: id[:strings.IndexByte(id, '/')], Repo: slug}
	for _, r := range releases {
		if r.DownloadURL == "" {
			continue
		}
		out.Releases = append(out.Releases, source.Release{
			Tag:        r.Version,
			Prerelease: !r.Stable,
			Assets:     []source.Asset{{Name: assetName(slug, r.Version), URL: r.DownloadURL}},
		})
	}
	return out, nil
}

// ResolveDownload returns the download URL for an asset's release matching version,
// or the newest stable release when version is empty (or unmatched).
func ResolveDownload(ctx context.Context, id, version string) (string, error) {
	releases, err := Releases(ctx, id)
	if err != nil {
		return "", err
	}
	if version != "" {
		for _, r := range releases {
			if r.Version == version && r.DownloadURL != "" {
				return r.DownloadURL, nil
			}
		}
	}
	rel, ok := PickStable(releases)
	if !ok || rel.DownloadURL == "" {
		return "", fmt.Errorf("store asset %q has no installable release", id)
	}
	return rel.DownloadURL, nil
}

// assetName is the filename a store release's zip is stored/installed under. It must
// end in .zip so the installer's zip path and the archive both handle it.
func assetName(slug, version string) string {
	name := slug
	if version != "" {
		name += "-" + version
	}
	return name + ".zip"
}

package search

import (
	"context"
	"net/url"
	"strconv"

	"gdaddon/internal/store"
)

// assetStoreBase is the root of the new Godot Asset Store
// (https://store.godotengine.org), the replacement for the legacy Asset Library.
const assetStoreBase = "https://store.godotengine.org"

// storePerPage is the page size the store's search API returns (the hit count per
// /api/v1/search/query/ response); used to derive the page count from the total.
const storePerPage = 24

// assetStore is the new Godot Asset Store backend.
//
// It speaks the store's JSON API (the same one the in-editor AssetLib browser uses,
// see godot's editor/asset_library/asset_library_editor_plugin.cpp):
//   - Search:  /api/v1/search/query/ — ranks/filters by query, honors page and the
//     engine-version filter (compatibility).
//   - Detail:  /api/v1/assets/<publisher>/<slug>/ — name, description, repo source.
//   - Release: /api/v1/releases/<publisher>/<slug>/ — the store-hosted .zip + version.
type assetStore struct{}

func (assetStore) Name() string { return "Asset Store" }

// AssetURL returns the canonical store URL for an asset id ("<publisher>/<slug>"),
// the stable identity pinned in the manifest for a store install (see
// store.IsStoreURL). Implementing AssetURLer marks this source as installable as a
// store asset rather than a git repo.
func (assetStore) AssetURL(id string) string { return store.AssetURL(id) }

// Search queries /api/v1/search/query/. type=0 selects addons (1 is templates). The
// API is 1-based and omits page on the first page, so page (0-indexed from the
// caller) is sent as page+1 only when paging past the first. godotVersion, when set,
// becomes the compatibility filter.
func (assetStore) Search(ctx context.Context, query, godotVersion string, page int) (*Page, error) {
	endpoint := assetStoreBase + "/api/v1/search/query/?query=" + url.QueryEscape(query) + "&type=0"
	if page > 0 {
		endpoint += "&page=" + strconv.Itoa(page+1)
	}
	if godotVersion != "" {
		endpoint += "&compatibility=" + url.QueryEscape(godotVersion)
	}

	var raw struct {
		Count string `json:"count"`
		Hits  []struct {
			Asset struct {
				Name        string `json:"name"`
				Slug        string `json:"slug"`
				LicenseType string `json:"license_type"`
				PriceCent   int    `json:"price_cent"`
				Publisher   struct {
					Name string `json:"name"`
					Slug string `json:"slug"`
				} `json:"publisher"`
				Tags []struct {
					DisplayName string `json:"display_name"`
				} `json:"tags"`
			} `json:"asset"`
		} `json:"hits"`
	}
	if err := getJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}

	out := &Page{Page: page}
	for _, h := range raw.Hits {
		a := h.Asset
		s := Summary{
			ID:     a.Publisher.Slug + "/" + a.Slug, // the detail-endpoint key
			Title:  a.Name,
			Author: a.Publisher.Name,
			Cost:   a.LicenseType,
		}
		if len(a.Tags) > 0 {
			s.Category = a.Tags[0].DisplayName
		}
		out.Results = append(out.Results, s)
	}

	out.TotalItems, _ = strconv.Atoi(raw.Count)
	out.Pages = (out.TotalItems + storePerPage - 1) / storePerPage
	if out.Pages < 1 {
		out.Pages = 1
	}
	return out, nil
}

// Detail resolves the asset's repo URL (its source) and latest stable release. id is
// the "<publisher>/<slug>" produced by Search. BrowseURL may be empty (paid/direct
// assets with no repo); DownloadURL is the store-hosted release .zip when a release
// exists. A failed/empty releases fetch is non-fatal — DownloadURL is just left
// blank. The caller decides what to do when both are empty.
func (assetStore) Detail(ctx context.Context, id string) (*Detail, error) {
	var asset struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Source      string `json:"source"`
		LicenseType string `json:"license_type"`
		Publisher   struct {
			Name string `json:"name"`
		} `json:"publisher"`
	}
	if err := getJSON(ctx, assetStoreBase+"/api/v1/assets/"+id+"/", &asset); err != nil {
		return nil, err
	}

	d := &Detail{
		Summary: Summary{
			ID:     id,
			Title:  asset.Name,
			Author: asset.Publisher.Name,
			Cost:   asset.LicenseType,
		},
		BrowseURL:   asset.Source,
		Description: asset.Description,
	}

	if releases, err := store.Releases(ctx, id); err == nil {
		if rel, ok := store.PickStable(releases); ok {
			d.DownloadURL = rel.DownloadURL
			d.VersionString = rel.Version
			d.GodotVersion = rel.MinGodotVersion
		}
	}

	return d, nil
}

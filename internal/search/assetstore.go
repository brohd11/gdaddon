package search

import (
	"context"
	"net/url"
	"strconv"

	"github.com/brohd11/gdaddon/internal/restrule"
	"github.com/brohd11/gdaddon/internal/store"
)

// assetStoreBase is the root of the new Godot Asset Store
// (https://store.godotengine.org), the replacement for the legacy Asset Library.
const assetStoreBase = "https://store.godotengine.org"

// storePerPage is the page size the store's search API returns (the hit count per
// /api/v1/search/query/ response); used to derive the page count from the total.
const storePerPage = 24

// assetStore is the Godot Asset Store backend, using the store's JSON API (as the editor's
// AssetLib does):
//   - Search:  /api/v1/search/query/ (query, page, engine-version filter)
//   - Detail:  /api/v1/assets/<publisher>/<slug>/ (name, description, repo)
//   - Release: /api/v1/releases/<publisher>/<slug>/ (store-hosted .zip and version)
type assetStore struct{}

func (assetStore) Name() string { return "Asset Store" }

// AssetURL returns the canonical store url for "<publisher>/<slug>", pinned in the
// manifest. Implementing AssetURLer makes results installable as store assets.
func (assetStore) AssetURL(id string) string { return store.AssetURL(id) }

// Search queries /api/v1/search/query/ (type=0: addons). Pages are 1-based and omitted on
// the first, so page+1 is sent only past it. godotVersion filters compatibility.
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
	if err := restrule.GetJSON(ctx, endpoint, &raw); err != nil {
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

// Detail resolves an asset's repo url and latest stable release zip. Either may be empty
// (a paid asset has no repo); a failed releases fetch just leaves DownloadURL empty.
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
	if err := restrule.GetJSON(ctx, assetStoreBase+"/api/v1/assets/"+id+"/", &asset); err != nil {
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

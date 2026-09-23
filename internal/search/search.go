// Package search queries Godot asset sources for addons. Each backend implements Source
// with shared result shapes; registering it in Sources adds it to the source selector.
package search

import (
	"context"

	"github.com/brohd11/gdaddon/internal/config"
)

// Summary is one search-result row. The list endpoint returns no repo/download
// URL — that only comes from Detail.
type Summary struct {
	ID            string
	Title         string
	Author        string
	Category      string
	Cost          string
	GodotVersion  string
	VersionString string
}

// Page is one page of search results plus the pagination bounds.
type Page struct {
	Results    []Summary
	Page       int // current page (0-indexed)
	Pages      int // total pages available
	TotalItems int
}

// Detail adds the per-asset fields only the detail endpoint returns — notably
// BrowseURL, the repo URL used to prefill the New Plugin form.
type Detail struct {
	Summary
	BrowseURL   string
	DownloadURL string
	Description string
}

// Source is one searchable backend. godotVersion ("4.3") filters by engine version; the
// Asset Library returns only a small legacy set without it. Sources may ignore it.
type Source interface {
	Name() string // display label for the source selector
	Search(ctx context.Context, query, godotVersion string, page int) (*Page, error)
	Detail(ctx context.Context, id string) (*Detail, error) // resolves the repo URL for prefill
}

// AssetURLer is implemented by sources whose assets install as store assets (a canonical
// store url), not git repos; the search flow offers "Add store asset" only for them.
type AssetURLer interface {
	AssetURL(id string) string
}

// Sources lists the backends in display order: the JSON sources from sources.yml (or the
// defaults), then the Asset Store. The first is the default. Misconfigured sources are
// skipped.
func Sources() []Source {
	var srcs []Source
	for _, sc := range config.Sources() {
		if cs := (configSource{cfg: sc}); cs.validate() == nil {
			srcs = append(srcs, cs)
		}
	}
	return append(srcs, assetStore{})
}

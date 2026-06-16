// Package search queries Godot asset sources for installable addons. Each
// backend implements Source; the shapes (Summary/Page/Detail) are
// source-agnostic so the TUI's search flow stays unchanged when a new backend
// (e.g. the upcoming Asset Store) is added — register it in Sources and it
// shows up in the source selector automatically.
package search

import "context"

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

// Source is one searchable asset backend.
//
// godotVersion is the engine version to filter results by (e.g. "4.3"). The
// Asset Library returns only a small legacy set when it's empty, so the caller
// detects the project's version and passes it; sources that don't filter by
// engine version may ignore it.
type Source interface {
	Name() string // display label for the source selector
	Search(ctx context.Context, query, godotVersion string, page int) (*Page, error)
	Detail(ctx context.Context, id string) (*Detail, error) // resolves the repo URL for prefill
}

// Sources is the registry of available backends, in display order. Append a new
// Source here to expose it in the search tab's source selector.
func Sources() []Source {
	return []Source{assetLib{}, assetStore{}}
}

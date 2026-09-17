// Package search is the Search tab: query a Godot asset source, browse results,
// and hand a chosen asset to the shared New Plugin flow with its repo URL
// prefilled. The actual querying lives in the source-agnostic internal/search
// package (imported here as searchpkg); adding a new backend there makes it
// appear in this tab's source selector with no changes here.
package search

import (
	"github.com/brohd11/gdaddon/internal/config"
	searchpkg "github.com/brohd11/gdaddon/internal/search"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/list"
)

// NewSearchScreen builds the Search tab's entry menu.
func NewSearchScreen(sh *core.Shared) *components.RootListScreen {
	return components.NewRootList(searchItems(), appctx.RootListOpts(sh, "Search"))
}

func searchItems() []list.Item {
	return []list.Item{
		components.Item{
			Name: "⌕ New search",
			Desc: "search a Godot asset source for an addon to add",
			Pick: func(sh *core.Shared) core.Action {
				c := appctx.Of(sh)
				return core.Push(newQueryScreen(defaultSource(), detectGodotVersion(c.ProjectRoot), c.LastSearchQuery))
			},
		},
	}
}

// defaultSource is the saved last-used source (config.yml: last_search_source) when
// it still matches a registered backend, otherwise the first registered backend.
func defaultSource() searchpkg.Source {
	srcs := searchpkg.Sources()
	if cfg, err := config.Load(); err == nil && cfg.LastSearchSource != "" {
		for _, s := range srcs {
			if s.Name() == cfg.LastSearchSource {
				return s
			}
		}
	}
	return srcs[0]
}

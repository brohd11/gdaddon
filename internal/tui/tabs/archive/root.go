// Package archive is the Archive tab: the locally archived packages, browsable by repo and
// version, where packages can be removed.
package archive

import (
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	pck "github.com/brohd11/gdaddon/internal/tui/flows/packages"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
)

// archiveTitle is the list's base Title; the active sort mode is appended.
const archiveTitle = "Archived Packages"

// archiveSortModes is the Archive tab's sort cycle: repo A→Z then Z→A (archived
// repos carry no install state, so there's no status grouping).
var archiveSortModes = []appctx.SortMode{appctx.SortAlpha, appctx.SortReverse}

// NewArchiveScreen builds the Archive tab from domain rows and refresh/sort callbacks.
func NewArchiveScreen(sh *core.Shared) *components.RootListScreen {
	mode := appctx.SortAlpha
	var screen *components.RootListScreen
	opts := appctx.RootListOpts(sh, appctx.SortTitle(archiveTitle, mode))
	opts.Help = []key.Binding{core.FullHint("sort", appctx.AppKeys.Sort)}
	opts.OnKey = func(sh *core.Shared, k string, _ list.Item) (core.Action, bool) {
		if !core.MatchKey(k, appctx.AppKeys.Sort) {
			return core.Action{}, false
		}
		appctx.CycleSort(screen.List(), &mode, archiveSortModes, archiveTitle,
			func(mode appctx.SortMode) []list.Item { return archiveItems(mode) })
		return core.Action{}, true
	}
	opts.Refresh = func(sh *core.Shared, payload any) ([]list.Item, bool) {
		if _, ok := payload.(appctx.ArchiveDirty); !ok {
			return nil, false
		}
		appctx.Of(sh).RefreshArchive()
		return archiveItems(mode), true
	}
	screen = components.NewRootList(archiveItems(mode), opts)
	return screen
}

// archiveItems builds the repo rows, ordered per mode (the underlying
// pck.RepoItems returns them ID-sorted; the sort toggle re-orders by row Title).
func archiveItems(mode appctx.SortMode) []list.Item {
	items := pck.RepoItems(archiveOpts)
	appctx.SortItemsByTitle(items, mode == appctx.SortReverse)
	return items
}

// archiveOpts is the Archive tab's browse config: the local archive, no HEAD, with the
// per-package Remove menu as its endpoint.
var archiveOpts = pck.BrowseOpts{Source: pck.SourceArchive, Endpoint: newPackageSubmenu}

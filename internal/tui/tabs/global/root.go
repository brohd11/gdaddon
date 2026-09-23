// Package global is the Global tab: the user's global plugin list, with a per-plugin
// submenu.
package global

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
)

// globalItem is one global entry carried into the submenu commands; the rows themselves
// are components.Item values.
type globalItem struct {
	// name is the key every write addresses; display is the label. Removing by label would
	// miss identity-keyed entries.
	name, display, url, path, version, tag string
	kind                                   addon.Kind
}

// entry rebuilds the addon.Addon this row came from, for the label and for the import
// that copies it into a project manifest.
func (g globalItem) entry() addon.Addon {
	return addon.Addon{Name: g.name, Display: g.display, URL: g.url, Path: g.path, Version: g.version, Tag: g.tag, Kind: g.kind}
}

// label is the row and heading text: the entry's own name, else its key's slug.
func (g globalItem) label() string { return g.entry().Label() }

// globalTitle is the list's base Title; the active sort mode is appended.
const globalTitle = "Global Plugins"

// globalSortModes is the Global tab's sort cycle: name A→Z then Z→A. There's no
// install-state grouping here (these rows carry no state).
var globalSortModes = []appctx.SortMode{appctx.SortAlpha, appctx.SortReverse}

// NewGlobalScreen builds the Global tab from domain rows and refresh/sort callbacks.
func NewGlobalScreen(sh *core.Shared) *components.RootListScreen {
	mode := appctx.SortAlpha
	var screen *components.RootListScreen
	opts := appctx.RootListOpts(sh, appctx.SortTitle(globalTitle, mode))
	opts.Help = []key.Binding{core.FullHint("sort", appctx.AppKeys.Sort)}
	opts.OnKey = func(sh *core.Shared, k string, _ list.Item) (core.Action, bool) {
		if !core.MatchKey(k, appctx.AppKeys.Sort) {
			return core.Action{}, false
		}
		appctx.CycleSort(screen.List(), &mode, globalSortModes, globalTitle,
			func(mode appctx.SortMode) []list.Item { return globalItems(sh, mode) })
		return core.Action{}, true
	}
	opts.Refresh = func(sh *core.Shared, payload any) ([]list.Item, bool) {
		if _, ok := payload.(appctx.GlobalDirty); !ok {
			return nil, false
		}
		appctx.Of(sh).RefreshGlobal()
		return globalItems(sh, mode), true
	}
	screen = components.NewRootList(globalItems(sh, mode), opts)
	return screen
}

// globalItems builds the Global rows, sorted per mode, or one hint row when empty.
func globalItems(sh *core.Shared, mode appctx.SortMode) []list.Item {
	var items []list.Item
	if path, err := addon.GlobalListPath(); err == nil {
		if addons, err := addon.Parse(path); err == nil {
			for _, a := range addons {
				g := globalItem{name: a.Name, display: a.Display, url: a.URL, path: a.Path, version: a.Version, tag: a.Tag, kind: a.Kind}
				items = append(items, components.Item{
					Name: g.label(),
					Desc: g.url,
					Pick: func(sh *core.Shared) core.Action { return core.Push(newSubmenuScreen(g, sh)) },
				})
			}
		}
	}
	items = components.EnsurePlaceholder(items, "(no global plugins yet)", "add one via Actions → New Plugin → Global")
	appctx.SortItemsByTitle(items, mode == appctx.SortReverse)
	return items
}

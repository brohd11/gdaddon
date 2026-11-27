// Package global is the Global tab: the user's global plugin list as a tree of category
// folders, with a per-plugin submenu.
package global

import (
	"sort"
	"strings"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/widgets"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
)

// globalItem is one global entry carried into the submenu commands; the rows themselves
// are tree leaves.
type globalItem struct {
	// name is the key every write addresses; display is the label. Removing by label would
	// miss identity-keyed entries.
	name, display, url, path, version, tag, category string
	kind                                             addon.Kind
}

// entry rebuilds the addon.Addon this row came from, for the label and for the import
// that copies it into a project manifest (which drops the category).
func (g globalItem) entry() addon.Addon {
	return addon.Addon{Name: g.name, Display: g.display, URL: g.url, Path: g.path, Version: g.version, Tag: g.tag, Kind: g.kind, Category: g.category}
}

// label is the row and heading text: the entry's own name, else its key's slug.
func (g globalItem) label() string { return g.entry().Label() }

// globalTitle is the tree's base Title; the active sort mode is appended.
const globalTitle = "Global Plugins"

// globalSortModes is the Global tab's sort cycle: name A→Z then Z→A, applied to folders
// and plugins at every level. There's no install-state grouping here (these rows carry no
// state).
var globalSortModes = []appctx.SortMode{appctx.SortAlpha, appctx.SortReverse}

// pluginIDPrefix starts a plugin leaf's node ID (folders use widgets.CategoryFolderPrefix);
// folds follow IDs.
const pluginIDPrefix = "plugin:"

// NewGlobalScreen builds the Global tab. Folders start collapsed; the session's folds live
// on the Ctx, so they survive the root being rebuilt.
func NewGlobalScreen(sh *core.Shared) *components.RootTreeScreen {
	mode := appctx.SortAlpha
	var screen *components.RootTreeScreen
	screen = components.NewRootTree(globalNodes(mode), components.RootTreeOpts{
		Title: appctx.SortTitle(globalTitle, mode),
		Help:  []key.Binding{core.FullHint("sort", appctx.AppKeys.Sort)},
		Folds: appctx.Of(sh).GlobalTreeFolds(),
		OnKey: func(sh *core.Shared, k string, _ components.TreeNode) (core.Action, bool) {
			if !core.MatchKey(k, appctx.AppKeys.Sort) {
				return core.Action{}, false
			}
			mode = components.NextSort(mode, globalSortModes)
			screen.SetNodes(globalNodes(mode))
			screen.SetTitle(appctx.SortTitle(globalTitle, mode))
			return core.Action{}, true
		},
		Refresh: func(sh *core.Shared, payload any) ([]components.TreeNode, bool) {
			if _, ok := payload.(appctx.GlobalDirty); !ok {
				return nil, false
			}
			appctx.Of(sh).RefreshGlobal()
			return globalNodes(mode), true
		},
	})
	return screen
}

// globalNodes builds the Global tree from the global list, sorted per mode, or one hint
// leaf when empty.
func globalNodes(mode appctx.SortMode) []components.TreeNode {
	var addons []addon.Addon
	if path, err := addon.GlobalListPath(); err == nil {
		addons, _ = addon.Parse(path)
	}
	if len(addons) == 0 {
		return []components.TreeNode{{ID: "placeholder", Item: components.CompactItem{
			Name: "(no global plugins yet)", Suffix: "add one via Actions → New Plugin → Global",
		}}}
	}
	reverse := mode == appctx.SortReverse
	items := make([]globalItem, 0, len(addons))
	for _, a := range addons {
		items = append(items, globalItem{name: a.Name, display: a.Display, url: a.URL, path: a.Path, version: a.Version, tag: a.Tag, kind: a.Kind, category: a.Category})
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := strings.ToLower(items[i].label()), strings.ToLower(items[j].label())
		if reverse {
			return a > b
		}
		return a < b
	})
	leaves := make([]widgets.CategoryLeaf, 0, len(items))
	for _, g := range items {
		leaves = append(leaves, widgets.CategoryLeaf{
			ID:       pluginIDPrefix + g.name,
			Category: g.category,
			Item: components.CompactItem{
				Name:   g.label(),
				Suffix: source.ShortRepo(g.url),
				Pick:   func(sh *core.Shared) core.Action { return core.Push(newSubmenuScreen(g, sh)) },
			},
		})
	}
	return widgets.CategoryTree(leaves, reverse)
}

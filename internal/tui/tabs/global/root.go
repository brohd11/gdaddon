// Package global is the Global tab: the user's global plugin list as a tree of category
// folders, with a per-plugin submenu.
package global

import (
	"sort"
	"strconv"
	"strings"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

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

// Node ID prefixes keep a folder and a plugin with the same text apart; folds follow IDs.
const (
	folderIDPrefix = "cat:"
	pluginIDPrefix = "plugin:"
)

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
	root := newFolder("", "")
	for _, a := range addons {
		g := globalItem{name: a.Name, display: a.Display, url: a.URL, path: a.Path, version: a.Version, tag: a.Tag, kind: a.Kind, category: a.Category}
		f := root
		for _, seg := range a.CategoryPath() {
			f = f.child(seg)
		}
		f.leaves = append(f.leaves, g)
	}
	return root.nodes(mode == appctx.SortReverse)
}

// folder is one category level while building the tree.
type folder struct {
	name, id string
	folders  map[string]*folder
	leaves   []globalItem
}

func newFolder(name, id string) *folder {
	return &folder{name: name, id: id, folders: map[string]*folder{}}
}

// child returns the named subfolder, creating it.
func (f *folder) child(name string) *folder {
	if c, ok := f.folders[name]; ok {
		return c
	}
	id := name
	if f.id != "" {
		id = f.id + "/" + name
	}
	c := newFolder(name, id)
	f.folders[name] = c
	return c
}

// count is the number of plugins at and below f.
func (f *folder) count() int {
	n := len(f.leaves)
	for _, c := range f.folders {
		n += c.count()
	}
	return n
}

// nodes renders f's contents: subfolders first, then plugins, each by name (reverse flips).
func (f *folder) nodes(reverse bool) []components.TreeNode {
	less := func(a, b string) bool {
		a, b = strings.ToLower(a), strings.ToLower(b)
		if reverse {
			return a > b
		}
		return a < b
	}
	subs := make([]*folder, 0, len(f.folders))
	for _, c := range f.folders {
		subs = append(subs, c)
	}
	sort.Slice(subs, func(i, j int) bool { return less(subs[i].name, subs[j].name) })
	leaves := append([]globalItem(nil), f.leaves...)
	sort.SliceStable(leaves, func(i, j int) bool { return less(leaves[i].label(), leaves[j].label()) })

	out := make([]components.TreeNode, 0, len(subs)+len(leaves))
	for _, c := range subs {
		out = append(out, components.TreeNode{
			ID:        folderIDPrefix + c.id,
			Item:      components.CompactItem{Name: c.name, Suffix: strconv.Itoa(c.count())},
			Children:  c.nodes(reverse),
			Collapsed: true,
		})
	}
	for _, g := range leaves {
		out = append(out, components.TreeNode{
			ID: pluginIDPrefix + g.name,
			Item: components.CompactItem{
				Name:   g.label(),
				Suffix: g.url,
				Pick:   func(sh *core.Shared) core.Action { return core.Push(newSubmenuScreen(g, sh)) },
			},
		})
	}
	return out
}

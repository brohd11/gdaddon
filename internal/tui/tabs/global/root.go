// Package global is the Global tab: a listing of the user's global plugin list
// (~/.gdaddon/plugins.yml). Selecting a plugin opens a per-plugin command submenu
// (currently just Import to Project).
package global

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// globalItem is one entry from the global plugin list, carried into the per-plugin
// submenu commands (Import / Remove). It is a payload, not a list row — the rows
// are self-dispatching components.Item values built in globalItems.
type globalItem struct {
	// name is the manifest key — what every write to plugins.yml addresses — while
	// display is the entry's own name. Keep them apart: removing by the rendered label
	// would miss an identity-keyed entry entirely.
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

// GlobalScreen is the Global tab root.
type GlobalScreen struct {
	list list.Model
	sort appctx.SortMode
}

var _ core.Filterer = (*GlobalScreen)(nil)
var _ core.Receiver = (*GlobalScreen)(nil)
var _ core.Crumber = (*GlobalScreen)(nil)

// CrumbLabel anchors the breadcrumb at the Global root.
func (s *GlobalScreen) CrumbLabel(bool) string { return "Tab" }

func NewGlobalScreen(sh *core.Shared) *GlobalScreen {
	l := core.NewSelectList(globalItems(sh, appctx.SortAlpha), appctx.SortTitle(globalTitle, appctx.SortAlpha))
	return &GlobalScreen{list: l}
}

// globalItems reads ~/.gdaddon/plugins.yml as self-dispatching rows, ordered per
// mode: each Pick opens that plugin's submenu. An empty/missing list shows an inert
// hint row.
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

func (s *GlobalScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *GlobalScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *GlobalScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	if k, ok := msg.(tea.KeyPressMsg); ok && !s.Filtering() && core.MatchKey(k.String(), appctx.AppKeys.Sort) {
		appctx.CycleSort(&s.list, &s.sort, globalSortModes, globalTitle,
			func(m appctx.SortMode) []list.Item { return globalItems(sh, m) })
		return s, core.Action{}
	}
	return s, components.RootUpdate(sh, &s.list, msg)
}

func (s *GlobalScreen) View(*core.Shared) string     { return core.RenderList(s.list) }
func (s *GlobalScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpTabbed) }

// Receive rebuilds the global list from disk on a GlobalDirty broadcast (after an
// add/remove), so the Global tab reflects the change. The status line and any focus
// switch are composed at the call site (core.Seq).
func (s *GlobalScreen) Receive(sh *core.Shared, payload any) core.Action {
	if _, ok := payload.(appctx.GlobalDirty); ok {
		appctx.Of(sh).RefreshGlobal()
		s.list.SetItems(globalItems(sh, s.sort))
	}
	return core.Action{}
}

func (s *GlobalScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

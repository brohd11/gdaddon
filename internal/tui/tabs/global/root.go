// Package global is the Global tab: a listing of the user's global plugin list
// (~/.gdaddon/plugins.yml). Selecting a plugin opens a per-plugin command submenu
// (currently just Import to Project).
package global

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// globalItem is one entry from the global plugin list, carried into the per-plugin
// submenu commands (Import / Remove). It is a payload, not a list row — the rows
// are self-dispatching components.Item values built in globalItems.
type globalItem struct{ name, url, path string }

// GlobalScreen is the Global tab root.
type GlobalScreen struct{ list list.Model }

var _ core.Filterer = (*GlobalScreen)(nil)
var _ core.Receiver = (*GlobalScreen)(nil)

func NewGlobalScreen() *GlobalScreen {
	return &GlobalScreen{list: core.NewSelectList(globalItems(), "Global Plugins")}
}

// globalItems reads ~/.gdaddon/plugins.yml as self-dispatching rows: each Pick
// opens that plugin's submenu. An empty/missing list shows an inert hint row.
func globalItems() []list.Item {
	var items []list.Item
	if path, err := addon.GlobalListPath(); err == nil {
		if addons, err := addon.Parse(path); err == nil {
			for _, a := range addons {
				g := globalItem{name: a.Name, url: a.URL, path: a.Path}
				items = append(items, components.Item{
					Name: g.name,
					Desc: g.url,
					Pick: func(sh *core.Shared) (tea.Msg, tea.Cmd) { return core.Push(newSubmenuScreen(g)), nil },
				})
			}
		}
	}
	if len(items) == 0 {
		items = append(items, components.Item{
			Name: "(no global plugins yet)",
			Desc: "add one via Actions → New Plugin → Global",
		})
	}
	return items
}

func (s *GlobalScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *GlobalScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *GlobalScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Msg, tea.Cmd) {
	m, c := components.RootUpdate(sh, &s.list, msg)
	return s, m, c
}

func (s *GlobalScreen) View(*core.Shared) string     { return s.list.View() }
func (s *GlobalScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpTabbed) }

// Receive rebuilds the global list from disk on a GlobalDirty broadcast (after an
// add/remove), so the Global tab reflects the change. When the event is focused it
// returns ShowTab so the router makes this tab active at its root.
func (s *GlobalScreen) Receive(sh *core.Shared, payload any) tea.Msg {
	d, ok := payload.(appctx.GlobalDirty)
	if !ok {
		return nil
	}
	sh.SetStatus(d.Status)
	s.list.SetItems(globalItems())
	if d.Focus {
		return core.ShowTab(appctx.TitleGlobal)
	}
	return nil
}

func (s *GlobalScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

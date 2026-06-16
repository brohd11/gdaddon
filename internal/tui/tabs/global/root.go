// Package global is the Global tab: a read-only listing of the user's global
// plugin list (~/.gdaddon/plugins.yml). It is scaffolding for future global
// commands — rows have no actions yet.
package global

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// globalItem is one entry from the global plugin list.
type globalItem struct{ name, url string }

func (i globalItem) Title() string       { return i.name }
func (i globalItem) FilterValue() string { return i.name }
func (i globalItem) Description() string { return i.url }

// GlobalScreen is the Global tab root.
type GlobalScreen struct{ list list.Model }

var _ core.Filterer = (*GlobalScreen)(nil)

func NewGlobalScreen() *GlobalScreen {
	return &GlobalScreen{list: core.NewSelectList(globalItems(), "Global Plugins")}
}

// globalItems reads ~/.gdaddon/plugins.yml; an empty/missing list shows a hint row.
func globalItems() []list.Item {
	var items []list.Item
	if path, err := addon.GlobalListPath(); err == nil {
		if addons, err := addon.Parse(path); err == nil {
			for _, a := range addons {
				items = append(items, globalItem{name: a.Name, url: a.URL})
			}
		}
	}
	if len(items) == 0 {
		items = append(items, globalItem{
			name: "(no global plugins yet)",
			url:  "add one via Actions → New Plugin → Global",
		})
	}
	return items
}

func (s *GlobalScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *GlobalScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *GlobalScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	if s.Filtering() {
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		return s, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "q" {
		return s, tea.Quit
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *GlobalScreen) View(*core.Shared) string     { return s.list.View() }
func (s *GlobalScreen) HelpView(*core.Shared) string { return core.RootHelp(s.list, core.HelpTabbed) }

func (s *GlobalScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

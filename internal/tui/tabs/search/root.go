// Package search is the Search tab: query a Godot asset source, browse results,
// and hand a chosen asset to the shared New Plugin flow with its repo URL
// prefilled. The actual querying lives in the source-agnostic internal/search
// package (imported here as searchpkg); adding a new backend there makes it
// appear in this tab's source selector with no changes here.
package search

import (
	searchpkg "gdaddon/internal/search"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// SearchScreen is the Search tab root. As a tab root it quits on q rather than
// popping.
type SearchScreen struct {
	list list.Model
}

var _ core.Filterer = (*SearchScreen)(nil)

func NewSearchScreen() *SearchScreen {
	return &SearchScreen{list: core.NewSelectList(searchItems(), "Search")}
}

func searchItems() []list.Item {
	return []list.Item{
		components.Item{
			Name: "⌕ New search",
			Desc: "search a Godot asset source for an addon to add",
			Pick: func(sh *core.Shared) tea.Cmd {
				return core.Push(newQueryScreen(defaultSource(), detectGodotVersion(sh.ProjectRoot)))
			},
		},
	}
}

// defaultSource is the first registered backend (today: the Asset Library).
func defaultSource() searchpkg.Source { return searchpkg.Sources()[0] }

func (s *SearchScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *SearchScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *SearchScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	if s.Filtering() {
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		return s, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		k := key.String()
		switch {
		case core.MatchKey(k, core.Keys.Quit):
			return s, tea.Quit
		case core.MatchKey(k, core.Keys.Select):
			if it, ok := s.list.SelectedItem().(components.Item); ok && it.Pick != nil {
				sh.StatusMsg = ""
				return s, it.Pick(sh)
			}
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *SearchScreen) View(*core.Shared) string     { return s.list.View() }
func (s *SearchScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpTabbed) }

func (s *SearchScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

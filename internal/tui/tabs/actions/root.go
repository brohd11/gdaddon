package actions

import (
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// actionsScreen is the Actions tab (reached with [ / ]): install all, new plugin,
// import plugin. As a tab root it quits on q rather than popping.
type ActionsScreen struct {
	list list.Model
}

var _ core.Filterer = (*ActionsScreen)(nil)

func NewActionsScreen() *ActionsScreen {
	return &ActionsScreen{list: core.NewSelectList(actionItems(), "Actions")}
}

func (s *ActionsScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *ActionsScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *ActionsScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
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

func (s *ActionsScreen) View(*core.Shared) string     { return s.list.View() }
func (s *ActionsScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpTabbed) }

func (s *ActionsScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

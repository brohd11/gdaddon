package actions

import (
	"gdaddon/internal/addon"
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
		switch key.String() {
		case "q":
			return s, tea.Quit
		case "enter":
			a, ok := s.list.SelectedItem().(actionItem)
			if !ok {
				return s, nil
			}
			switch a.kind {
			case actInstallAll:
				return s, core.Push(newInstallAllTask())
			case actNewPlugin:
				sh.StatusMsg = ""
				return s, core.Push(NewNewPluginForm())
			case actImportPlugin:
				sh.StatusMsg = ""
				return s.startImport(sh)
			}
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

// startImport loads the global plugin list and opens the picker, or reports an
// empty/missing list and returns to browse.
func (s *ActionsScreen) startImport(sh *core.Shared) (core.Screen, tea.Cmd) {
	path, err := addon.GlobalListPath()
	var addons []addon.Addon
	if err == nil {
		addons, err = addon.Parse(path)
	}
	if err != nil || len(addons) == 0 {
		sh.StatusMsg = "no global plugins yet — add one via New Plugin → Global"
		return s, core.ResetToRoot()
	}
	return s, core.Push(newImportScreen(addons))
}

func (s *ActionsScreen) View(*core.Shared) string     { return s.list.View() }
func (s *ActionsScreen) HelpView(*core.Shared) string { return core.RootHelp(s.list, core.HelpTabbed) }

func (s *ActionsScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

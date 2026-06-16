package tui

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// actionsScreen is the Actions tab (reached with [ / ]): install all, new plugin,
// import plugin. As a tab root it quits on q rather than popping.
type actionsScreen struct {
	list list.Model
}

var _ core.Filterer = (*actionsScreen)(nil)

func newActionsScreen() *actionsScreen {
	return &actionsScreen{list: core.NewSelectList(actionItems(), "Actions")}
}

func (s *actionsScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *actionsScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *actionsScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
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
				return s, core.Push(newNewPluginForm())
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
func (s *actionsScreen) startImport(sh *core.Shared) (core.Screen, tea.Cmd) {
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

func (s *actionsScreen) View(*core.Shared) string     { return s.list.View() }
func (s *actionsScreen) HelpView(*core.Shared) string { return core.RootHelp(s.list, core.HelpTabbed) }

func (s *actionsScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

package tui

import (
	"gdaddon/internal/addon"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// actionsScreen is the Actions tab (reached with [ / ]): install all, new plugin,
// import plugin. As a tab root it quits on q rather than popping.
type actionsScreen struct {
	list list.Model
}

var _ filterer = (*actionsScreen)(nil)

func newActionsScreen() *actionsScreen {
	return &actionsScreen{list: newSelectList(actionItems(), "Actions")}
}

func (s *actionsScreen) Init(*shared) tea.Cmd { return nil }

func (s *actionsScreen) filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *actionsScreen) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
	if s.filtering() {
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
				return s, push(newInstallAllTask())
			case actNewPlugin:
				sh.statusMsg = ""
				return s, push(newNewPluginForm())
			case actImportPlugin:
				sh.statusMsg = ""
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
func (s *actionsScreen) startImport(sh *shared) (screen, tea.Cmd) {
	path, err := addon.GlobalListPath()
	var addons []addon.Addon
	if err == nil {
		addons, err = addon.Parse(path)
	}
	if err != nil || len(addons) == 0 {
		sh.statusMsg = "no global plugins yet — add one via New Plugin → Global"
		return s, resetToRoot()
	}
	return s, push(newImportScreen(addons))
}

func (s *actionsScreen) View(*shared) string     { return s.list.View() }
func (s *actionsScreen) HelpView(*shared) string { return rootHelp(s.list, helpTabbed) }

func (s *actionsScreen) SetSize(sh *shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

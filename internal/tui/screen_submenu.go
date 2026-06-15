package tui

import (
	"gdaddon/internal/addon"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// submenuScreen is the leaf picker: a release's assets, or HEAD's branches.
type submenuScreen struct {
	list          list.Model
	selected      addon.Addon
	selectedLocal string
}

var _ filterer = (*submenuScreen)(nil)

func newSubmenuScreen(selected addon.Addon, local string, items []list.Item, title string) *submenuScreen {
	return &submenuScreen{
		list:          newSelectList(items, title, archiveKey),
		selected:      selected,
		selectedLocal: local,
	}
}

func (s *submenuScreen) Init(*shared) tea.Cmd { return nil }

func (s *submenuScreen) filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *submenuScreen) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
	if s.filtering() {
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		return s, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			return s, pop()
		case "enter":
			v, ok := s.list.SelectedItem().(versionItem)
			if !ok {
				return s, nil
			}
			return s, push(newInstallConfirm(s.selected, s.selectedLocal, v))
		case "a":
			cs, status, ok := buildArchiveConfirm(s.selected, s.selectedLocal, s.list.SelectedItem())
			if status != "" {
				sh.statusMsg = status
			}
			if !ok {
				return s, nil
			}
			return s, push(cs)
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *submenuScreen) View(*shared) string     { return s.list.View() }
func (s *submenuScreen) HelpView(*shared) string { return helpView(s.list) }

func (s *submenuScreen) SetSize(sh *shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

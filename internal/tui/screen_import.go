package tui

import (
	"gdaddon/internal/addon"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// importScreen picks an entry from the global plugin list to copy into the
// project manifest.
type importScreen struct {
	list list.Model
}

var _ filterer = (*importScreen)(nil)

func newImportScreen(addons []addon.Addon) *importScreen {
	items := make([]list.Item, 0, len(addons))
	for _, a := range addons {
		items = append(items, importItem{name: a.Name, url: a.URL, path: a.Path})
	}
	return &importScreen{list: newSelectList(items, "Import Plugin")}
}

func (s *importScreen) Init(*shared) tea.Cmd { return nil }

func (s *importScreen) filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *importScreen) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
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
			return s.commit(sh)
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

// commit copies the selected global entry into the project manifest (deriving the
// install path), then unwinds to browse with the list rebuilt.
func (s *importScreen) commit(sh *shared) (screen, tea.Cmd) {
	sel, ok := s.list.SelectedItem().(importItem)
	if !ok {
		return s, nil
	}
	if err := addon.AddEntry(sh.manifestPath, sel.name, sel.url, sel.path); err != nil {
		sh.statusMsg = "error: " + err.Error()
		return s, resetToRoot()
	}
	return s, tea.Batch(resetToRoot(), reloadCmd(sh, "imported "+sel.name))
}

func (s *importScreen) View(*shared) string     { return s.list.View() }
func (s *importScreen) HelpView(*shared) string { return helpView(s.list) }

func (s *importScreen) SetSize(sh *shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

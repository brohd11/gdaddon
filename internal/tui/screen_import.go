package tui

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// importScreen picks an entry from the global plugin list to copy into the
// project manifest.
type importScreen struct {
	list list.Model
}

var _ core.Filterer = (*importScreen)(nil)

func newImportScreen(addons []addon.Addon) *importScreen {
	items := make([]list.Item, 0, len(addons))
	for _, a := range addons {
		items = append(items, importItem{name: a.Name, url: a.URL, path: a.Path})
	}
	return &importScreen{list: core.NewSelectList(items, "Import Plugin")}
}

func (s *importScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *importScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *importScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	if s.Filtering() {
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		return s, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			return s, core.Pop()
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
func (s *importScreen) commit(sh *core.Shared) (core.Screen, tea.Cmd) {
	sel, ok := s.list.SelectedItem().(importItem)
	if !ok {
		return s, nil
	}
	if err := addon.AddEntry(sh.ManifestPath, sel.name, sel.url, sel.path); err != nil {
		sh.StatusMsg = "error: " + err.Error()
		return s, core.ResetToRoot()
	}
	return s, tea.Batch(core.ResetToRoot(), reloadCmd(sh, "imported "+sel.name))
}

func (s *importScreen) View(*core.Shared) string     { return s.list.View() }
func (s *importScreen) HelpView(*core.Shared) string { return core.HelpView(s.list) }

func (s *importScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

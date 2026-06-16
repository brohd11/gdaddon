// Package archive is the Archive tab: a listing of the locally-archived packages
// (~/.gdaddon/archive). Selecting a repo drills into its archived versions (the
// versions.go flow, repo-level), where a package can be removed from the archive.
package archive

import (
	"fmt"

	arch "gdaddon/internal/archive"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// ArchiveScreen is the Archive tab root. Its rows are self-dispatching
// components.Item values, so the only hand-written list logic in the whole tab is
// this screen's Update (the pushed pickers run their items' Pick closures).
type ArchiveScreen struct{ list list.Model }

var _ core.Filterer = (*ArchiveScreen)(nil)
var _ core.RootHandler = (*ArchiveScreen)(nil)

func NewArchiveScreen() *ArchiveScreen {
	return &ArchiveScreen{list: core.NewSelectList(repoItems(), "Archived Packages")}
}

// repoItems reads every archived repo; an empty/missing archive shows a hint row.
// Each repo's Pick opens its versions picker.
func repoItems() []list.Item {
	repos, _ := arch.Repos()
	var items []list.Item
	for _, repo := range repos {
		repo := repo // capture per row
		items = append(items, components.Item{
			Name: repo.ID,
			Desc: fmt.Sprintf("%d version(s)", len(repo.Releases)),
			Pick: func(sh *core.Shared) tea.Cmd { return core.Push(newVersionsPicker(repo)) },
		})
	}
	if len(items) == 0 {
		items = append(items, components.Item{
			Name: "(nothing archived yet)",
			Desc: "archive a package via Project → an addon → Archive",
		})
	}
	return items
}

func (s *ArchiveScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *ArchiveScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *ArchiveScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
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

func (s *ArchiveScreen) View(*core.Shared) string     { return s.list.View() }
func (s *ArchiveScreen) HelpView(*core.Shared) string { return core.RootHelp(s.list, core.HelpTabbed) }

// HandleRoot rebuilds the list from disk on a MsgRefresh targeting the archive
// (after a package removal), so the tab reflects the change. Routed here by the router.
func (s *ArchiveScreen) HandleRoot(sh *core.Shared, msg tea.Msg) bool {
	m, ok := msg.(core.MsgRefresh)
	if !ok || m.Target != core.RefreshArchive {
		return false
	}
	sh.StatusMsg = m.Status
	s.list.SetItems(repoItems())
	return true
}

func (s *ArchiveScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

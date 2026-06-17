// Package archive is the Archive tab: a listing of the locally-archived packages
// (~/.gdaddon/archive). Selecting a repo drills into its archived versions (the
// versions.go flow, repo-level), where a package can be removed from the archive.
package archive

import (
	"fmt"

	arch "gdaddon/internal/archive"
	"gdaddon/internal/tui/appctx"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// ArchiveScreen is the Archive tab root. Its rows are self-dispatching
// components.Item values, so it has no bespoke list logic: Update delegates to the
// shared components.RootUpdate (the pushed pickers run their items' Pick closures).
type ArchiveScreen struct{ list list.Model }

var _ core.Filterer = (*ArchiveScreen)(nil)
var _ core.Receiver = (*ArchiveScreen)(nil)

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
			Pick: func(sh *core.Shared) (tea.Msg, tea.Cmd) { return core.Push(newVersionsPicker(repo)), nil },
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

func (s *ArchiveScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Msg, tea.Cmd) {
	m, c := components.RootUpdate(sh, &s.list, msg)
	return s, m, c
}

func (s *ArchiveScreen) View(*core.Shared) string     { return s.list.View() }
func (s *ArchiveScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpTabbed) }

// Receive rebuilds the list from disk on an ArchiveDirty broadcast (after a package
// removal), so the tab reflects the change. A removal in this tab is focused; a removal
// triggered as a side effect of a global remove is not (Focus false), so the Archive
// tab reloads silently while focus stays on the Global tab.
func (s *ArchiveScreen) Receive(sh *core.Shared, payload any) tea.Msg {
	d, ok := payload.(appctx.ArchiveDirty)
	if !ok {
		return nil
	}
	sh.SetStatus(d.Status)
	s.list.SetItems(repoItems())
	if d.Focus {
		return core.ShowTab(appctx.TitleArchive)
	}
	return nil
}

func (s *ArchiveScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

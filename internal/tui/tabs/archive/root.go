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
	return s, components.RootUpdate(sh, &s.list, msg)
}

func (s *ArchiveScreen) View(*core.Shared) string     { return s.list.View() }
func (s *ArchiveScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpTabbed) }

// HandleRoot rebuilds the list from disk on a MsgRefresh targeting the archive
// (after a package removal), so the tab reflects the change. Routed here by the router.
func (s *ArchiveScreen) HandleRoot(sh *core.Shared, msg tea.Msg) bool {
	m, ok := msg.(core.MsgRefresh)
	if !ok || m.Target != appctx.Archive {
		return false
	}
	sh.StatusMsg = m.Status
	s.list.SetItems(repoItems())
	return true
}

func (s *ArchiveScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

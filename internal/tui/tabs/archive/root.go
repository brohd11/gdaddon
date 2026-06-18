// Package archive is the Archive tab: a listing of the locally-archived packages
// (~/.gdaddon/archive). Selecting a repo drills into its archived versions (the
// versions.go flow, repo-level), where a package can be removed from the archive.
package archive

import (
	"gdaddon/internal/tui/appctx"
	pck "gdaddon/internal/tui/flows/packages"

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
	return &ArchiveScreen{list: core.NewSelectList(pck.RepoItems(archiveOpts), "Archived Packages")}
}

// archiveOpts is the Archive tab's browse config: the local archive, no HEAD, with the
// per-package Remove menu as its endpoint.
var archiveOpts = pck.BrowseOpts{Source: pck.SourceArchive, Endpoint: newPackageSubmenu}

func (s *ArchiveScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *ArchiveScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *ArchiveScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	return s, components.RootUpdate(sh, &s.list, msg)
}

func (s *ArchiveScreen) View(*core.Shared) string     { return s.list.View() }
func (s *ArchiveScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpTabbed) }

// Receive rebuilds the list from disk on an ArchiveDirty broadcast (after a package
// removal), so the tab reflects the change. The status line and any focus switch are
// composed at the call site (core.Seq): a removal in this tab focuses Archive, while a
// removal triggered as a side effect of a global remove reloads silently.
func (s *ArchiveScreen) Receive(sh *core.Shared, payload any) core.Action {
	if _, ok := payload.(appctx.ArchiveDirty); ok {
		appctx.Of(sh).RefreshArchive()
		s.list.SetItems(pck.RepoItems(archiveOpts))
	}
	return core.Action{}
}

func (s *ArchiveScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

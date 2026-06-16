package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// versionsScreen is the top-level version picker: HEAD plus one row per release.
type versionsScreen struct {
	list          list.Model
	selected      addon.Addon
	selectedLocal string
	ghListing     *source.Listing // raw upstream listing (nil when offline/delisted)
	listing       *source.Listing // ghListing merged with archived packages
}

var _ core.Filterer = (*versionsScreen)(nil)
var _ core.Relister = (*versionsScreen)(nil)

func newVersionsScreen(selected addon.Addon, local string, gh, listing *source.Listing) *versionsScreen {
	l := core.NewSelectList(versionTopItems(listing), core.HeaderTitle(selected.Name, local, "Versions"), archiveKey)
	return &versionsScreen{list: l, selected: selected, selectedLocal: local, ghListing: gh, listing: listing}
}

// newReleasesLoading builds the loading screen for an addon's release fetch. Its
// onResult folds in archived packages and opens the versions screen (or pops on a
// hard failure with no archive fallback) — the merge/next-screen logic the generic
// loadingScreen no longer owns.
func newReleasesLoading(a addon.Addon, local string) *components.LoadingScreen {
	onResult := func(sh *core.Shared, msg tea.Msg) tea.Cmd {
		m, ok := msg.(releasesMsg)
		if !ok {
			return nil
		}
		var archived []source.Release
		if repoID, err := source.RepoID(a.URL); err == nil {
			archived, _ = archive.List(repoID)
		}
		if m.err != nil && len(archived) == 0 {
			sh.StatusMsg = "error: " + m.err.Error()
			return core.Pop()
		}
		gh := m.listing
		listing := archive.Merge(cloneListing(gh), archived)
		return core.Replace(newVersionsScreen(a, local, gh, listing))
	}
	return components.NewLoadingScreen(core.HeaderTitle(a.Name, local, ""), "fetching versions…", fetchReleases(a.URL), onResult)
}

// newBranchesLoading builds the loading screen for a HEAD/branch fetch. Its onResult
// opens the branch submenu (or unwinds on error / empty).
func newBranchesLoading(a addon.Addon, local string) *components.LoadingScreen {
	onResult := func(sh *core.Shared, msg tea.Msg) tea.Cmd {
		m, ok := msg.(branchesMsg)
		if !ok {
			return nil
		}
		if m.err != nil {
			sh.StatusMsg = "error: " + m.err.Error()
			return core.ResetToRoot()
		}
		if len(m.branches) == 0 {
			sh.StatusMsg = "no branches found"
			return core.Pop()
		}
		sub := newInstallPicker(a, local, branchItems(m.branches), core.HeaderTitle(a.Name, local, "Branches"))
		return core.Replace(sub)
	}
	return components.NewLoadingScreen(core.HeaderTitle(a.Name, local, ""), "fetching branches…", fetchBranches(a.URL), onResult)
}

func (s *versionsScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *versionsScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *versionsScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
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
			return s.selectVersion()
		case "a":
			return s.archiveSelection(sh, s.list.SelectedItem())
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

// selectVersion handles enter: HEAD opens the branch submenu (after a fetch), a
// single-asset release goes straight to confirm, a multi-asset release opens the
// asset submenu.
func (s *versionsScreen) selectVersion() (core.Screen, tea.Cmd) {
	switch sel := s.list.SelectedItem().(type) {
	case headItem:
		return s, core.Push(newBranchesLoading(s.selected, s.selectedLocal))
	case releaseItem:
		if len(sel.rel.Assets) == 1 {
			a := sel.rel.Assets[0]
			pick := versionItem{tag: sel.rel.Tag, asset: a, prerelease: sel.rel.Prerelease, archived: isArchived(a)}
			return s, core.Push(newInstallConfirm(s.selected, s.selectedLocal, pick))
		}
		sub := newInstallPicker(s.selected, s.selectedLocal,
			assetItems(sel.rel), core.HeaderTitle(s.selected.Name, s.selectedLocal, "Assets "+sel.rel.Tag))
		return s, core.Push(sub)
	}
	return s, nil
}

// archiveSelection pushes the archive confirm for the selected version-list item.
// A release archives all its remote assets; a leaf asset/branch archives just
// that one. HEAD (no concrete asset) is ignored.
func (s *versionsScreen) archiveSelection(sh *core.Shared, sel list.Item) (core.Screen, tea.Cmd) {
	cs, status, ok := buildArchiveConfirm(s.selected, s.selectedLocal, sel)
	if status != "" {
		sh.StatusMsg = status
	}
	if !ok {
		return s, nil
	}
	return s, core.Push(cs)
}

// relist re-merges the archive into the listing and rebuilds the rows, so newly
// archived packages appear (called after an archive task finishes).
func (s *versionsScreen) Relist() {
	if repoID, err := source.RepoID(s.selected.URL); err == nil {
		archived, _ := archive.List(repoID)
		s.listing = archive.Merge(cloneListing(s.ghListing), archived)
		s.list.SetItems(versionTopItems(s.listing))
	}
}

func (s *versionsScreen) View(*core.Shared) string     { return s.list.View() }
func (s *versionsScreen) HelpView(*core.Shared) string { return core.HelpView(s.list) }

func (s *versionsScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

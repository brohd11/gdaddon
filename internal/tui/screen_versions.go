package tui

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"

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

var _ filterer = (*versionsScreen)(nil)

func newVersionsScreen(selected addon.Addon, local string, gh, listing *source.Listing) *versionsScreen {
	l := newSelectList(versionTopItems(listing), headerTitle(selected.Name, local, "Versions"), archiveKey)
	return &versionsScreen{list: l, selected: selected, selectedLocal: local, ghListing: gh, listing: listing}
}

func (s *versionsScreen) Init(*shared) tea.Cmd { return nil }

func (s *versionsScreen) filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *versionsScreen) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
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
func (s *versionsScreen) selectVersion() (screen, tea.Cmd) {
	switch sel := s.list.SelectedItem().(type) {
	case headItem:
		ld := newLoadingScreen(s.selected, s.selectedLocal, "fetching branches…", fetchBranches(s.selected.URL))
		return s, push(ld)
	case releaseItem:
		if len(sel.rel.Assets) == 1 {
			a := sel.rel.Assets[0]
			pick := versionItem{tag: sel.rel.Tag, asset: a, prerelease: sel.rel.Prerelease, archived: isArchived(a)}
			return s, push(newInstallConfirm(s.selected, s.selectedLocal, pick))
		}
		sub := newSubmenuScreen(s.selected, s.selectedLocal,
			assetItems(sel.rel), headerTitle(s.selected.Name, s.selectedLocal, "Assets "+sel.rel.Tag))
		return s, push(sub)
	}
	return s, nil
}

// archiveSelection pushes the archive confirm for the selected version-list item.
// A release archives all its remote assets; a leaf asset/branch archives just
// that one. HEAD (no concrete asset) is ignored.
func (s *versionsScreen) archiveSelection(sh *shared, sel list.Item) (screen, tea.Cmd) {
	cs, status, ok := buildArchiveConfirm(s.selected, s.selectedLocal, sel)
	if status != "" {
		sh.statusMsg = status
	}
	if !ok {
		return s, nil
	}
	return s, push(cs)
}

// relist re-merges the archive into the listing and rebuilds the rows, so newly
// archived packages appear (called after an archive task finishes).
func (s *versionsScreen) relist() {
	if repoID, err := source.RepoID(s.selected.URL); err == nil {
		archived, _ := archive.List(repoID)
		s.listing = archive.Merge(cloneListing(s.ghListing), archived)
		s.list.SetItems(versionTopItems(s.listing))
	}
}

func (s *versionsScreen) View(*shared) string     { return s.list.View() }
func (s *versionsScreen) HelpView(*shared) string { return helpView(s.list) }

func (s *versionsScreen) SetSize(sh *shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

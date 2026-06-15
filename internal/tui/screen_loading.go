package tui

import (
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// loadingScreen is the non-interactive spinner shown while an upstream fetch is
// in flight. It owns the fetch result: releasesMsg replaces it with the versions
// screen; branchesMsg replaces it with the branch submenu.
type loadingScreen struct {
	selected      addon.Addon
	selectedLocal string
	label         string
	cmd           tea.Cmd // the fetch command, run on Init
}

func newLoadingScreen(selected addon.Addon, local, label string, cmd tea.Cmd) *loadingScreen {
	return &loadingScreen{selected: selected, selectedLocal: local, label: label, cmd: cmd}
}

func (s *loadingScreen) Init(sh *shared) tea.Cmd {
	return tea.Batch(sh.spinner.Tick, s.cmd)
}

func (s *loadingScreen) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case releasesMsg:
		// Fold in locally archived packages. If the upstream fetch failed but the
		// archive has entries, fall back to an archive-only listing — the whole
		// point of the archive (a delisted/offline repo can still be reinstalled).
		var archived []source.Release
		if repoID, err := source.RepoID(s.selected.URL); err == nil {
			archived, _ = archive.List(repoID)
		}
		if msg.err != nil && len(archived) == 0 {
			sh.statusMsg = "error: " + msg.err.Error()
			return s, pop()
		}
		gh := msg.listing // nil when the fetch failed
		listing := archive.Merge(cloneListing(gh), archived)
		return s, replace(newVersionsScreen(s.selected, s.selectedLocal, gh, listing))

	case branchesMsg:
		if msg.err != nil {
			sh.statusMsg = "error: " + msg.err.Error()
			return s, resetToRoot()
		}
		if len(msg.branches) == 0 {
			sh.statusMsg = "no branches found"
			return s, pop()
		}
		sub := newSubmenuScreen(s.selected, s.selectedLocal,
			branchItems(msg.branches), headerTitle(s.selected.Name, s.selectedLocal, "Branches"))
		return s, replace(sub)
	}
	return s, nil
}

func (s *loadingScreen) View(sh *shared) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		renderTitleBar(headerTitle(s.selected.Name, s.selectedLocal, "")),
		fmt.Sprintf("  %s %s", sh.spinner.View(), s.label))
}

func (s *loadingScreen) HelpView(sh *shared) string {
	return sh.noteHelp("non-interactive · working…")
}

func (s *loadingScreen) SetSize(*shared, int, int) {}

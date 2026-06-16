package global

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/core"

	tea "github.com/charmbracelet/bubbletea"
)

// reloadCmd re-inspects the manifest and returns MsgRootRefresh so the router
// switches to the Project tab and rebuilds its list with the imported row.
func reloadCmd(sh *core.Shared, status string) tea.Cmd {
	manifestPath, projectRoot := sh.ManifestPath, sh.ProjectRoot
	return func() tea.Msg {
		statuses, _ := addon.Inspect(manifestPath, projectRoot)
		return core.MsgRootRefresh{Status: status, Statuses: statuses, Rebuild: true}
	}
}

// commitRemove removes the plugin from the global list, plus its archived packages
// when the chosen mode is "global + archive". On success it refreshes the Global
// tab (GlobalRefresh) so the removed row disappears.
func commitRemove(sh *core.Shared, g globalItem, mode int) tea.Cmd {
	if mode == removeGlobalArchive {
		if repoID, err := source.RepoID(g.url); err == nil {
			if err := archive.RemoveRepo(repoID); err != nil {
				sh.StatusMsg = "error: " + err.Error()
				return core.ResetToRoot()
			}
		}
		// A non-github url has no archive to remove; fall through to the entry removal.
	}

	globalPath, err := addon.GlobalListPath()
	if err == nil {
		err = addon.RemoveEntry(globalPath, g.name)
	}
	if err != nil {
		sh.StatusMsg = "error: " + err.Error()
		return core.ResetToRoot()
	}
	return core.GlobalRefresh("removed " + g.name)
}

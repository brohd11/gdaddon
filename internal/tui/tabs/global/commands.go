package global

import (
	"gdaddon/internal/addon"
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

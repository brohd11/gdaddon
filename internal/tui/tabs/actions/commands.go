package actions

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/core"

	tea "github.com/charmbracelet/bubbletea"
)

// finishInstallAllCmd re-inspects after a batch install for the router to apply to
// the Browse tab.
func finishInstallAllCmd(sh *core.Shared) tea.Cmd {
	manifestPath, projectRoot := sh.ManifestPath, sh.ProjectRoot
	return func() tea.Msg {
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return core.MsgRootRefresh{Status: "install complete"}
		}
		return core.MsgRootRefresh{Status: "install complete", Statuses: statuses}
	}
}

// reloadCmd re-inspects the manifest and returns MsgRootRefresh so the router
// rebuilds the Browse list (after a row was added) and sets the status line.
func reloadCmd(sh *core.Shared, status string) tea.Cmd {
	manifestPath, projectRoot := sh.ManifestPath, sh.ProjectRoot
	return func() tea.Msg {
		statuses, _ := addon.Inspect(manifestPath, projectRoot)
		return core.MsgRootRefresh{Status: status, Statuses: statuses, Rebuild: true}
	}
}

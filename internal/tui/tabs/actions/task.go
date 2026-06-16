package actions

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	tea "github.com/charmbracelet/bubbletea"
)

// newInstallAllTask runs a batch install of everything in the manifest, then lands
// the result on the Browse tab (RootRefresh reloads it from the manifest).
func newInstallAllTask() *components.TaskScreen {
	run := func(sh *core.Shared, report func(string, ...any), done chan<- core.InstallEvent) {
		statuses, err := addon.Inspect(sh.ManifestPath, sh.ProjectRoot)
		if err != nil {
			report("error: %v", err)
		} else {
			_ = addon.InstallAll(sh.ManifestPath, statuses, sh.ProjectRoot, report)
		}
		done <- core.InstallEvent{Done: true}
	}
	onDone := func(sh *core.Shared, ev core.InstallEvent) tea.Cmd { return core.RootRefresh("install complete") }
	return components.NewTask("installing all addons…", run, onDone)
}

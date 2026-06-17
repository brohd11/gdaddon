package actions

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// newInstallAllTask runs a batch install of everything in the manifest, then lands
// the result on the Browse tab (a Project refresh reloads it from the manifest).
func newInstallAllTask() *components.TaskScreen {
	run := func(sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		c := appctx.Of(sh)
		statuses, err := addon.Inspect(c.ManifestPath, c.ProjectRoot)
		if err != nil {
			report("error: %v", err)
		} else {
			_ = addon.InstallAll(c.ManifestPath, statuses, c.ProjectRoot, report)
		}
		done <- core.TaskEvent{Done: true}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) tea.Cmd {
		return core.PropagateAll(appctx.ProjectDirty{Status: "install complete", Focus: true})
	}
	return components.NewTask("installing all addons…", run, onDone)
}

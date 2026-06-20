package actions

import (
	"context"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// newInstallAllTask runs a batch install of everything in the manifest, then lands
// the result on the Browse tab (a Project refresh reloads it from the manifest).
func newInstallAllTask() *components.TaskScreen {
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		c := appctx.Of(sh)
		statuses, err := addon.Inspect(c.ManifestPath, c.ProjectRoot)
		if err != nil {
			report("error: %v", err)
		} else {
			_ = addon.InstallAll(ctx, c.ManifestPath, statuses, c.ProjectRoot, report)
		}
		done <- core.TaskEvent{Done: true}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		return core.Seq(
			core.SetStatus("install complete"),
			core.PropagateAll(appctx.ProjectDirty{}),
			core.ShowTab(appctx.TitleProject),
		)
	}
	return components.NewTask("installing all addons…", run, onDone)
}

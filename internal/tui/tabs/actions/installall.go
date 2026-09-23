package actions

import (
	"context"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/list"
)

// newInstallUpdatePicker groups install, install with dependencies, and update-all.
func newInstallUpdatePicker(sh *core.Shared) core.Screen {
	items := []list.Item{
		components.Item{
			Name: "↧ Install All",
			Desc: "download and install everything per the manifest",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newInstallAllConfirm()) },
		},
		components.Item{
			Name: "↧ Install All + Deps",
			Desc: "install all, then resolve and install declared dependencies",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newInstallAllDepsConfirm(sh)) },
		},
		components.Item{
			Name: "⟳ Update All",
			Desc: "update installed addons to their latest release",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newUpdateAllLoading(sh)) },
		},
	}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Install/Update"})
}

func newInstallAllDepsConfirm(sh *core.Shared) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb: "Install + Deps",
		Text:  "Install all packages and resolve their dependencies?",
		OnYes: core.Push(newInstallAllDepsTask()),
	})
}

// newInstallAllDepsTask runs the recursive install, then shows the Project tab.
func newInstallAllDepsTask() *components.TaskScreen {
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		c := appctx.Of(sh)
		// Check the error so an aborted run is not reported as complete. No per-dependency
		// confirmer: the confirm before this task covers the run (the CLI vets one by one).
		outcomes, err := addon.InstallAllDeps(ctx, c.ManifestPath, c.ProjectRoot, nil, report)
		if err != nil {
			report("error: %v", err)
		}
		done <- core.TaskEvent{Done: true, Payload: outcomes}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		outcomes, _ := ev.Payload.([]addon.InstallOutcome)
		return finishBatch(sh, outcomes, "install complete")
	}
	return components.NewTask("installing all addons + dependencies…", run, onDone)
}

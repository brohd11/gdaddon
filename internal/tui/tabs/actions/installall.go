package actions

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newInstallUpdatePicker is the Actions ▸ Install/Update All submenu: plain install,
// install with recursive dependency resolution, or update-all. Each row pushes its
// existing flow, so the submenu is just a grouping.
func newInstallUpdatePicker(sh *core.Shared) core.Screen {
	items := []list.Item{
		components.Item{
			Name: "↧ Install All",
			Desc: "download and install everything per the manifest",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newInstallAllConfirm(sh)) },
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

func newInstallAllDepsConfirm(sh *core.Shared) *components.ConfirmScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb: "Install + Deps",
		Text:  "Install all packages and resolve their dependencies?",
		OnYes: core.Push(newInstallAllDepsTask()),
	})
}

// newInstallAllDepsTask runs the recursive install (install all → import declared
// dependencies → install → repeat until nothing new), then lands on the Project tab
// like the plain install-all task.
func newInstallAllDepsTask() *components.TaskScreen {
	run := func(sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		c := appctx.Of(sh)
		_ = addon.InstallAllDeps(c.ManifestPath, c.ProjectRoot, report)
		done <- core.TaskEvent{Done: true}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		return core.Seq(
			core.SetStatus("install complete"),
			core.PropagateAll(appctx.ProjectDirty{}),
			core.ShowTab(appctx.TitleProject),
		)
	}
	return components.NewTask("installing all addons + dependencies…", run, onDone)
}

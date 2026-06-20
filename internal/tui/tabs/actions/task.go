package actions

import (
	"context"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/postinstall"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// newInstallAllTask runs a batch install of everything in the manifest, then lands
// the result on the Browse tab (a Project refresh reloads it from the manifest).
func newInstallAllTask() *components.TaskScreen {
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		c := appctx.Of(sh)
		var outcomes []addon.InstallOutcome
		statuses, err := addon.Inspect(c.ManifestPath, c.ProjectRoot)
		if err != nil {
			report("error: %v", err)
		} else {
			outcomes, _ = addon.InstallAll(ctx, c.ManifestPath, statuses, c.ProjectRoot, report)
		}
		done <- core.TaskEvent{Done: true, Payload: outcomes}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		outcomes, _ := ev.Payload.([]addon.InstallOutcome)
		return finishBatch(sh, outcomes, "install complete")
	}
	return components.NewTask("installing all addons…", run, onDone)
}

// finishBatch routes a completed batch run: if any installed addon's path changed from
// its prior manifest path, hand off to the shared post-install location form queue;
// otherwise land on the Project tab as before (a refresh reloads it from the manifest).
func finishBatch(sh *core.Shared, outcomes []addon.InstallOutcome, doneStatus string) core.Action {
	targets := locationTargets(outcomes)
	if len(targets) == 0 {
		return core.Seq(
			core.SetStatus(doneStatus),
			core.PropagateAll(appctx.ProjectDirty{}),
			core.ShowTab(appctx.TitleProject),
		)
	}
	return core.Replace(postinstall.New(sh, targets))
}

// locationTargets keeps the batch outcomes whose install path changed (path-less first
// installs / relocations) — the ones worth confirming — as location-form targets.
func locationTargets(outcomes []addon.InstallOutcome) []postinstall.Target {
	var targets []postinstall.Target
	for _, o := range outcomes {
		if o.Path != "" && o.Path != o.PriorPath {
			targets = append(targets, postinstall.Target{Name: o.Name, URL: o.URL, Path: o.Path, Version: o.Version})
		}
	}
	return targets
}

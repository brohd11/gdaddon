package project

import (
	"context"
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/postinstall"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// The streaming task screen itself is the generic components.TaskScreen. These
// builders supply the run/onDone closures for each feature; install and install-all
// navigate away on completion, archive stays on the log until dismissed.

// installResult is the install task's terminating payload, carried opaquely through
// core.TaskEvent.Payload and read back in onDone.
type installResult struct{ Path, Version string }

func newInstallTask(selected addon.Addon, local string, pick versionItem) *components.TaskScreen {
	target := addon.Addon{Name: selected.Name, URL: pick.asset.URL, Path: selected.Path}
	if pick.clone {
		// Clone the canonical repo (.git url from the repo id), checking out the
		// chosen branch, instead of unzipping the branch archive.
		target.URL = "https://" + pick.repoID + ".git"
		target.Tag = pick.tag
		target.Clone = true
	}
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		res, err := addon.Install(ctx, target, appctx.Of(sh).ProjectRoot, report)
		done <- core.TaskEvent{Done: true, Err: err, Payload: installResult{Path: res.Path, Version: res.Version}}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		if ev.Err != nil {
			return core.Seq(
				core.SetStatusAndLog(fmt.Sprintf("[%s] error: %v", selected.Name, ev.Err)),
				core.SetStatusAndLog("install failed", true),
				core.ResetToRoot(),
			)
		}
		sh.Log(fmt.Sprintf("[%s] installed", selected.Name))
		res, _ := ev.Payload.(installResult)
		// Pin the resolved path immediately (matches the batch flows). When that path
		// differs from the entry's prior manifest path (a path-less or relocated
		// entry), hand off to the shared location form so the user can confirm/correct
		// it and optionally record it globally; a package shipping several addons
		// (res.Path == "") can't be tracked to one folder, so it finishes silently.
		status := pinInstall(appctx.Of(sh).ManifestPath, selected, pick, res.Path, res.Version)
		if res.Path != "" && res.Path != selected.Path {
			t := postinstall.Target{Name: selected.Name, URL: selected.URL, Path: res.Path, Version: res.Version}
			return core.Replace(postinstall.New(sh, []postinstall.Target{t}))
		}
		return core.Seq(
			core.SetStatus(status),
			core.PropagateAll(appctx.ProjectDirty{}),
			core.ShowTab(appctx.TitleProject),
		)
	}
	return components.NewTask("installing "+selected.Name+"…", run, onDone)
}

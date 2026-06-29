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
	if !pick.branch {
		// A real release tag (branch-HEAD archives have none): carry it so a
		// config-less package is stamped with a version.cfg on install.
		target.Tag = pick.tag
	}
	if pick.clone {
		// Clone the canonical repo (.git url from the repo id), checking out the
		// chosen branch, instead of unzipping the branch archive.
		target.URL = "https://" + pick.repoID + ".git"
		target.Tag = pick.tag
		target.Kind = addon.KindClone
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

// newStoreInstallTask installs the chosen Asset Store version. Store assets have no
// git asset/clone variants: the target carries the canonical store url + the picked
// version, and addon.Install (→ storeInstall) resolves that release's download and
// unzips it. On success it pins the chosen store version + resolved path (url left
// untouched) and lands on Project, handing off to the location form when the
// resolved path differs, exactly like newInstallTask.
func newStoreInstallTask(selected addon.Addon, local, version string) *components.TaskScreen {
	target := selected
	target.Version = version
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
		// Pin the chosen store version (the release identity) + resolved path; leave
		// url empty so the canonical store url is untouched.
		_ = addon.UpdateEntry(appctx.Of(sh).ManifestPath, selected.Name, "", res.Path, version, "")
		if res.Path != "" && res.Path != selected.Path {
			t := postinstall.Target{Name: selected.Name, URL: selected.URL, Path: res.Path, Version: version}
			return core.Replace(postinstall.New(sh, []postinstall.Target{t}))
		}
		return core.Seq(
			core.SetStatus("installed "+selected.Name),
			core.PropagateAll(appctx.ProjectDirty{}),
			core.ShowTab(appctx.TitleProject),
		)
	}
	return components.NewTask("installing "+selected.Name+"…", run, onDone)
}

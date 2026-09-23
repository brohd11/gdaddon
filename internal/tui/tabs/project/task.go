package project

import (
	"context"
	"fmt"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/postinstall"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// Task builders over components.TaskScreen: install tasks navigate away when done; archive
// stays on the log.

// The install task's final payload is the addon.InstallResult itself, read back in onDone.

// newInstallTaskScreen installs target and pins the result via pin, then opens the location
// form when the path differs from the prior one. A multi-folder package (no Path) just
// finishes.
func newInstallTaskScreen(selected addon.Addon, target addon.Addon, pin func(sh *core.Shared, res addon.InstallResult) (string, error)) *components.TaskScreen {
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		res, err := addon.Install(ctx, target, appctx.Of(sh).ProjectRoot, report)
		done <- core.TaskEvent{Done: true, Err: err, Payload: res}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		if ev.Err != nil {
			return core.Seq(
				core.SetStatusAndLog(fmt.Sprintf("[%s] error: %v", selected.Label(), ev.Err)),
				core.SetStatusAndLog("install failed", true),
				core.ResetToRoot(),
			)
		}
		sh.Log(fmt.Sprintf("[%s] installed", selected.Label()))
		res, _ := ev.Payload.(addon.InstallResult)
		status, err := pin(sh, res)
		if err != nil {
			// Installed on disk but the manifest pin failed: surface it (the success
			// line would lie) and skip the location form — it re-pins the same way.
			return core.SeqErr(err, core.PropagateAll(appctx.ProjectDirty{}), core.ShowTab(appctx.TitleProject))
		}
		if res.Path != "" && res.Path != selected.Path {
			t := postinstall.Target{Name: selected.Name, Display: selected.Display, URL: selected.URL, Path: res.Path, Version: res.Version}
			return core.Replace(postinstall.New(sh, []postinstall.Target{t}))
		}
		return core.Seq(
			core.SetStatus(status),
			core.PropagateAll(appctx.ProjectDirty{}),
			core.ShowTab(appctx.TitleProject),
		)
	}
	return components.NewTask("installing "+selected.Label()+"…", run, onDone)
}

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
		target.URL = addon.CloneURL(pick.repoID)
		target.Tag = pick.tag
		target.Kind = addon.KindClone
	}
	return newInstallTaskScreen(selected, target, func(sh *core.Shared, res addon.InstallResult) (string, error) {
		// Pin the resolved path immediately (matches the batch flows).
		return pinInstall(appctx.Of(sh).ManifestPath, selected, pick, res)
	})
}

// newStoreInstallTask installs an Asset Store version (store url, release as tag) and pins
// the installed version, tag and path.
func newStoreInstallTask(selected addon.Addon, local, version string) *components.TaskScreen {
	target := selected
	target.Tag = version
	return newInstallTaskScreen(selected, target, func(sh *core.Shared, res addon.InstallResult) (string, error) {
		// Pin the installed plugin.cfg version + the store release identity (tag) +
		// resolved path; leave url empty so the canonical store url is untouched.
		manifestPath := appctx.Of(sh).ManifestPath
		if err := addon.UpdateEntry(manifestPath, selected.Name, "", res.Path, res.Version, version); err != nil {
			return "", err
		}
		if err := addon.AdoptName(manifestPath, selected, res); err != nil {
			return "", err
		}
		return "installed " + selected.Label(), nil
	})
}

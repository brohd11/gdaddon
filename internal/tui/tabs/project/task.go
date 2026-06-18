package project

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"

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
	run := func(sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		res, err := addon.Install(target, appctx.Of(sh).ProjectRoot, report)
		done <- core.TaskEvent{Done: true, Err: err, Payload: installResult{Path: res.Path, Version: res.Version}}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		if ev.Err != nil {
			sh.Log(fmt.Sprintf("[%s] error: %v", selected.Name, ev.Err))
			sh.SetStatus("install failed")
			return core.ResetToRoot()
		}
		sh.Log(fmt.Sprintf("[%s] installed", selected.Name))
		res, _ := ev.Payload.(installResult)
		return core.Async(finishInstallCmd(sh, selected, pick, res.Path, res.Version))
	}
	return components.NewTask("installing "+selected.Name+"…", run, onDone)
}

func newArchiveTask(selected addon.Addon, tag, repoID string, assets []source.Asset) *components.TaskScreen {
	_ = selected
	run := func(sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		for _, a := range assets {
			report("downloading %s …", strings.TrimSuffix(a.Name, " - archived"))
			if err := archive.Archive(repoID, tag, a); err != nil {
				done <- core.TaskEvent{Done: true, Err: err}
				return
			}
		}
		done <- core.TaskEvent{Done: true}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		if ev.Err != nil {
			sh.Log("archive failed: " + ev.Err.Error())
			return core.Action{}
		}
		sh.Log("archived " + tag)
		// Reload the Archive tab so the freshly stored package shows up. Focus stays
		// on this Project stay-task (no ShowTab) — the broadcast is a silent reload.
		return core.PropagateAll(appctx.ArchiveDirty{})
	}
	onDismiss := func(sh *core.Shared) core.Action {
		sh.Chrome.Status.Clear()
		return core.PopTo() // back to the addon submenu (its command hub)
	}
	return components.NewStayTask("archiving "+tag+"…", "done — esc to go back", run, onDone, onDismiss)
}

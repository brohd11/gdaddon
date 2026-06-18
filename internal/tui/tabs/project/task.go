package project

import (
	"fmt"

	"gdaddon/internal/addon"
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
			return core.Seq(
				core.SetStatusAndLog(fmt.Sprintf("[%s] error: %v", selected.Name, ev.Err)),
				core.SetStatusAndLog("install failed", true),
				core.ResetToRoot(),
			)
		}
		sh.Log(fmt.Sprintf("[%s] installed", selected.Name))
		res, _ := ev.Payload.(installResult)
		return core.Async(finishInstallCmd(sh, selected, pick, res.Path, res.Version))
	}
	return components.NewTask("installing "+selected.Name+"…", run, onDone)
}


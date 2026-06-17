package project

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	"github.com/brohd/bubblestack/components"
	"github.com/brohd/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
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
	onDone := func(sh *core.Shared, ev core.TaskEvent) tea.Cmd {
		if ev.Err != nil {
			sh.AppendLog(fmt.Sprintf("[%s] error: %v", selected.Name, ev.Err))
			sh.StatusMsg = "install failed"
			return core.ResetToRoot()
		}
		sh.AppendLog(fmt.Sprintf("[%s] installed", selected.Name))
		res, _ := ev.Payload.(installResult)
		return finishInstallCmd(sh, selected, pick, res.Path, res.Version)
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
	onDone := func(sh *core.Shared, ev core.TaskEvent) tea.Cmd {
		if ev.Err != nil {
			sh.AppendLog("archive failed: " + ev.Err.Error())
		} else {
			sh.AppendLog("archived " + tag)
		}
		return nil
	}
	onDismiss := func(sh *core.Shared) tea.Cmd {
		sh.StatusMsg = ""
		return core.PopTo() // back to the addon submenu (its command hub)
	}
	return components.NewStayTask("archiving "+tag+"…", "done — esc to go back", run, onDone, onDismiss)
}

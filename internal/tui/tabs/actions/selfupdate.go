package actions

import (
	"context"
	"fmt"
	"time"

	"gdaddon/internal/selfupdate"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// selfUpdateCheckTimeout caps the release-listing fetch behind the loading screen so
// a slow or unreachable host can't hang it.
const selfUpdateCheckTimeout = 30 * time.Second

// selfUpdateInfoMsg carries the self-update check result from the background fetch to
// the loading screen's result handler.
type selfUpdateInfoMsg struct {
	info selfupdate.Info
	err  error
}

// newSelfUpdateLoading is the entry point of the Actions ▸ Update gdaddon flow:
// loading → confirm → task. It checks gdaddon's own repo off the UI thread; when an
// update exists it opens the confirm, otherwise it reports "up to date" and pops.
func newSelfUpdateLoading(sh *core.Shared) *components.LoadingScreen {
	version := appctx.Of(sh).Version
	cmd := func(parent context.Context) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, selfUpdateCheckTimeout)
			defer cancel()
			info, err := selfupdate.Check(ctx, version)
			return selfUpdateInfoMsg{info: info, err: err}
		}
	}
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		m, ok := msg.(selfUpdateInfoMsg)
		if !ok {
			return core.Action{}
		}
		if m.err != nil {
			return core.Seq(core.SetStatusAndLog("update check failed: "+m.err.Error()), core.Pop())
		}
		if !m.info.Available {
			return core.Seq(core.SetStatus("gdaddon is up to date"), core.Pop())
		}
		return core.Replace(newSelfUpdateConfirm(m.info))
	}
	return components.NewLoadingScreen("Update gdaddon", "checking for gdaddon update…", cmd, onResult)
}

// newSelfUpdateConfirm shows the pending update ("current → latest") and, on confirm,
// runs the download+install task.
func newSelfUpdateConfirm(info selfupdate.Info) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb: "Update gdaddon",
		Text:  fmt.Sprintf("Update gdaddon %s → %s?", info.Current, info.LatestTag),
		OnYes: core.Replace(newSelfUpdateTask(info)),
	})
}

// newSelfUpdateTask downloads and installs the new binary (to the running binary's
// managed location, or ~/.gdaddon/bin), then pops to the Actions root. The running
// process keeps the old code in memory, so it reports that a relaunch picks up the
// new binary.
func newSelfUpdateTask(info selfupdate.Info) *components.TaskScreen {
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		path, err := selfupdate.Apply(ctx, info, selfupdate.DefaultDest(), report)
		done <- core.TaskEvent{Done: true, Err: err, Payload: path}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		if ev.Err != nil {
			return core.Seq(
				core.SetStatusAndLog("update failed: "+ev.Err.Error(), true),
				core.Pop(),
			)
		}
		return core.Seq(
			core.SetStatusAndLog(fmt.Sprintf("updated to %s — relaunch gdaddon to use it", info.LatestTag), true),
			core.Pop(),
		)
	}
	return components.NewTask("updating gdaddon…", run, onDone)
}

package actions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// updateResolveTimeout caps the whole batch of release-listing fetches that build
// the update plan, so a slow or unreachable host can't hang the loading screen.
const updateResolveTimeout = 60 * time.Second

// updatePlansMsg carries the resolved update plans (and the addons skipped as
// ambiguous) from the background fetch to the loading screen's result handler.
type updatePlansMsg struct {
	plans   []addon.UpdatePlan
	skipped []addon.SkippedUpdate
}

// newUpdateAllLoading captures the manifest paths, then fetches every installed
// addon's latest release off the UI thread (resolveUpdatePlansCmd). When the
// listings come back it logs any addon skipped for having multiple packages, then
// either reports "up to date" and pops, or opens the confirm listing each
// "name old → new". It's the entry point of the Actions ▸ Update all flow:
// loading → confirm → task.
func newUpdateAllLoading(sh *core.Shared) *components.LoadingScreen {
	c := appctx.Of(sh)
	cmd := resolveUpdatePlansCmd(c.ManifestPath, c.ProjectRoot)
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		m, ok := msg.(updatePlansMsg)
		if !ok {
			return core.Action{}
		}
		for _, s := range m.skipped {
			sh.Log(fmt.Sprintf("[%s] update skipped: multiple packages (%s) — update manually", s.Name, s.Tag))
		}
		if len(m.plans) == 0 {
			status := "all installed addons are up to date"
			if len(m.skipped) > 0 {
				status = fmt.Sprintf("%d addon(s) need a manual update (multiple packages)", len(m.skipped))
			}
			return core.Seq(
				core.SetStatus(status),
				core.Pop(),
			)
		}
		return core.Replace(newUpdateAllConfirm(m.plans, m.skipped))
	}
	return components.NewLoadingScreen("Update All", "checking for updates…", cmd, onResult)
}

// resolveUpdatePlansCmd resolves the update plans off the UI thread via
// addon.ResolveUpdatePlans, capping the whole batch of release-listing fetches
// with updateResolveTimeout so a slow or unreachable host can't hang the loading
// screen.
func resolveUpdatePlansCmd(manifestPath, projectRoot string) func(context.Context) tea.Cmd {
	return func(parent context.Context) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, updateResolveTimeout)
			defer cancel()
			plans, skipped, err := addon.ResolveUpdatePlans(ctx, manifestPath, projectRoot)
			if err != nil {
				return updatePlansMsg{}
			}
			return updatePlansMsg{plans: plans, skipped: skipped}
		}
	}
}

// newUpdateAllConfirm lists the pending updates ("name old → new"), any addons skipped
// for having multiple packages, and, on confirm, runs the batch update task.
func newUpdateAllConfirm(plans []addon.UpdatePlan, skipped []addon.SkippedUpdate) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb: "Update All",
		Text:  updateAllBody(plans, skipped),
		OnYes: core.Replace(newUpdateAllTask(plans)),
	})
}

func updateAllBody(plans []addon.UpdatePlan, skipped []addon.SkippedUpdate) string {
	lines := make([]string, 0, len(plans)+len(skipped)+3)
	lines = append(lines, fmt.Sprintf("Update %d addon(s) to their latest release:\n", len(plans)))
	for _, p := range plans {
		old := p.OldVersion
		if old == "" {
			old = "unknown"
		}
		lines = append(lines, fmt.Sprintf("  %s   %s → %s", p.Addon.Name, old, p.NewTag))
	}
	if len(skipped) > 0 {
		lines = append(lines, "", "Skipped (multiple packages — update manually):")
		for _, s := range skipped {
			lines = append(lines, fmt.Sprintf("  %s   %s", s.Name, s.Tag))
		}
	}
	return strings.Join(lines, "\n")
}

// newUpdateAllTask installs each plan's target asset, then lands on the Project tab
// (a ProjectDirty reloads it from the updated manifest), mirroring the install-all
// task's completion.
func newUpdateAllTask(plans []addon.UpdatePlan) *components.TaskScreen {
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		c := appctx.Of(sh)
		outcomes, _ := addon.UpdateAll(ctx, c.ManifestPath, plans, c.ProjectRoot, report)
		done <- core.TaskEvent{Done: true, Payload: outcomes}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		outcomes, _ := ev.Payload.([]addon.InstallOutcome)
		return finishBatch(sh, outcomes, "update complete")
	}
	return components.NewTask("updating addons…", run, onDone)
}

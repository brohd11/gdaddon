package actions

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
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

// updatePlansMsg carries the resolved update plans from the background fetch to
// the loading screen's result handler.
type updatePlansMsg struct {
	plans []addon.UpdatePlan
}

// newUpdateAllLoading captures the manifest paths, then fetches every installed
// addon's latest release off the UI thread (resolveUpdatePlansCmd). When the
// listings come back it either reports "up to date" and pops, or opens the confirm
// listing each "name old → new". It's the entry point of the Actions ▸ Update all
// flow: loading → confirm → task.
func newUpdateAllLoading(sh *core.Shared) *components.LoadingScreen {
	c := appctx.Of(sh)
	cmd := resolveUpdatePlansCmd(c.ManifestPath, c.ProjectRoot)
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		m, ok := msg.(updatePlansMsg)
		if !ok {
			return core.Action{}
		}
		if len(m.plans) == 0 {
			return core.Seq(
				core.SetStatus("all installed addons are up to date"),
				core.Pop(),
			)
		}
		return core.Replace(newUpdateAllConfirm(m.plans))
	}
	return components.NewLoadingScreen("Update All", "checking for updates…", cmd, onResult)
}

// resolveUpdatePlansCmd inspects the manifest and, concurrently, resolves an
// update plan for each installed addon that has a newer release than the one
// installed. Not-installed or url-less entries are skipped (nothing to compare).
// The plans are name-sorted so the confirm list is deterministic.
func resolveUpdatePlansCmd(manifestPath, projectRoot string) func(context.Context) tea.Cmd {
	return func(parent context.Context) tea.Cmd {
		return func() tea.Msg {
			statuses, err := addon.Inspect(manifestPath, projectRoot)
			if err != nil {
				return updatePlansMsg{}
			}

			ctx, cancel := context.WithTimeout(parent, updateResolveTimeout)
			defer cancel()

			var mu sync.Mutex
			var wg sync.WaitGroup
			plans := make([]addon.UpdatePlan, 0)
			for _, s := range statuses {
				if !s.Present() || s.Addon.URL == "" {
					continue
				}
				wg.Add(1)
				go func(a addon.Addon, local string) {
					defer wg.Done()
					if plan, ok := addon.ResolveUpdate(ctx, a, local); ok {
						mu.Lock()
						plans = append(plans, plan)
						mu.Unlock()
					}
				}(s.Addon, s.LocalVersion)
			}
			wg.Wait()
			sort.Slice(plans, func(i, j int) bool { return plans[i].Addon.Name < plans[j].Addon.Name })
			return updatePlansMsg{plans: plans}
		}
	}
}

// newUpdateAllConfirm lists the pending updates ("name old → new") and, on confirm,
// runs the batch update task.
func newUpdateAllConfirm(plans []addon.UpdatePlan) *components.ConfirmScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb: "Update All",
		Text:  updateAllBody(plans),
		OnYes: core.Replace(newUpdateAllTask(plans)),
	})
}

func updateAllBody(plans []addon.UpdatePlan) string {
	lines := make([]string, 0, len(plans)+1)
	lines = append(lines, fmt.Sprintf("Update %d addon(s) to their latest release:\n", len(plans)))
	for _, p := range plans {
		old := p.OldVersion
		if old == "" {
			old = "unknown"
		}
		lines = append(lines, fmt.Sprintf("  %s   %s → %s", p.Addon.Name, old, p.NewTag))
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

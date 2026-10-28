package project

import (
	"context"
	"time"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/core"

	tea "charm.land/bubbletea/v2"
)

// updateChecksReady carries update-check results to the Project root as a broadcast, so the
// markers appear even from another tab.
type updateChecksReady struct {
	checks   map[string]addon.UpdateInfo
	stampErr error
}

// updateCheckTimeout caps the whole batch of release-listing fetches so a slow or
// unreachable host can't leave the check pending forever.
const updateCheckTimeout = 30 * time.Second

// checkUpdatesCmd checks each installed addon for a newer release off the UI thread and
// broadcasts the results. A check superseded before it starts (RefreshAll asks twice in
// one tick) skips its network work.
func checkUpdatesCmd(sh *core.Shared) tea.Cmd {
	c := appctx.Of(sh)
	manifestPath, projectRoot := c.ManifestPath, c.ProjectRoot
	gen := c.UpdateGen.Add(1)
	return func() tea.Msg {
		if c.UpdateGen.Load() != gen || manifestPath == "" {
			return nil
		}
		begin, stampErr := addon.BeginUpdateCheck(projectRoot, time.Now(), false)
		if !begin {
			return nil
		}
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			if stampErr != nil {
				return core.PropagateAll(updateChecksReady{stampErr: stampErr})
			}
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()

		checks := addon.CheckUpdates(ctx, statuses)
		return core.PropagateAll(updateChecksReady{checks: checks, stampErr: stampErr})
	}
}

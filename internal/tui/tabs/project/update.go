package project

import (
	"context"
	"sync"
	"time"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// updateChecksReady carries freshly computed per-addon update-check results back
// to the Project root via a PropagateAll broadcast, where they're cached on the
// context and the list is rebuilt so the "update available" markers appear. It
// reaches the cached root even when another tab is active.
type updateChecksReady struct {
	checks map[string]addon.UpdateInfo
}

// updateCheckTimeout caps the whole batch of release-listing fetches so a slow or
// unreachable host can't leave the check pending forever.
const updateCheckTimeout = 30 * time.Second

// checkUpdatesCmd fetches each installed addon's release listing off the UI
// thread and reports, per addon, whether a newer release than the pinned one
// exists. The manifest paths are captured up front; the inspect + network work
// runs inside the cmd so the list never blocks. Results ride back on a
// PropagateAll the root caches and renders. Not-installed or url-less entries are
// skipped (nothing to compare).
func checkUpdatesCmd(sh *core.Shared) tea.Cmd {
	c := appctx.Of(sh)
	manifestPath, projectRoot := c.ManifestPath, c.ProjectRoot
	return func() tea.Msg {
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()

		checks := make(map[string]addon.UpdateInfo)
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, s := range statuses {
			if !s.Present() || s.Addon.URL == "" {
				continue
			}
			wg.Add(1)
			go func(a addon.Addon) {
				defer wg.Done()
				info := addon.CheckUpdate(ctx, a)
				mu.Lock()
				checks[a.Name] = info
				mu.Unlock()
			}(s.Addon)
		}
		wg.Wait()
		return core.PropagateAll(updateChecksReady{checks: checks})
	}
}

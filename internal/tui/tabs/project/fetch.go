package project

import (
	"context"
	"time"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// fetchDone carries the results of a fetch-all pass back to the Project root via a
// PropagateAll broadcast, where they're logged and the list is rebuilt from the freshly
// updated refs. Like updateChecksReady it reaches the cached root even from another tab.
type fetchDone struct {
	results []addon.FetchResult
}

// fetchTimeout caps the whole fan-out, so an unreachable remote can't leave the fetch
// pending (and the root stuck marked as fetching) forever.
const fetchTimeout = 90 * time.Second

// fetchAllCmd runs `git fetch` in every present git checkout the manifest tracks — plus the
// project root's own repo, which is no manifest entry but is fetched unconditionally here —
// off the UI thread. It's the announced network step behind the ahead/behind markers: those
// counts are read locally on every refresh, but only a fetch can make them notice new
// upstream commits — so this is bound to a key rather than riding the inspect pass.
//
// The manifest paths and the cached root repo are captured up front and nothing touches
// Shared inside the goroutine (it belongs to the UI thread); the results ride back on the
// broadcast, the same shape as checkUpdatesCmd.
func fetchAllCmd(sh *core.Shared) tea.Cmd {
	c := appctx.Of(sh)
	manifestPath, projectRoot := c.ManifestPath, c.ProjectRoot
	root := c.RootRepo
	return func() tea.Msg {
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return core.PropagateAll(fetchDone{})
		}

		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		results := addon.FetchAll(ctx, statuses)
		if root != nil {
			// Same shape FetchAll builds per checkout, for the root repo the manifest
			// doesn't cover: fetch, then read the post-fetch divergence back.
			res := addon.FetchResult{Name: root.Name, Err: addon.GitFetch(ctx, root.Dir)}
			res.Sync = addon.GitSyncStatus(root.Dir)
			results = append(results, res)
		}

		return core.Seq(
			core.PropagateAll(fetchDone{results: results}),
			core.RefreshRoots(),
		)
	}
}

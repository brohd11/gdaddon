package project

import (
	"context"
	"fmt"
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

// fetchAllCmd runs `git fetch` in every present git checkout the manifest tracks, off the
// UI thread. It's the announced network step behind the ahead/behind markers: those counts
// are read locally on every refresh, but only a fetch can make them notice new upstream
// commits — so this is bound to a key rather than riding the inspect pass.
//
// The manifest paths are captured up front and nothing touches Shared inside the goroutine
// (it belongs to the UI thread); the results ride back on the broadcast, the same shape as
// checkUpdatesCmd.
func fetchAllCmd(sh *core.Shared) tea.Cmd {
	c := appctx.Of(sh)
	manifestPath, projectRoot := c.ManifestPath, c.ProjectRoot
	return func() tea.Msg {
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return core.PropagateAll(fetchDone{})
		}

		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		return core.Seq(
			core.PropagateAll(fetchDone{results: addon.FetchAll(ctx, statuses)}),
			core.RefreshRoots(),
		)
	}
}

// fetchLine describes one repo's fetch outcome for the output log.
func fetchLine(r addon.FetchResult) string {
	switch {
	case r.Err != nil:
		return fmt.Sprintf("[%s] fetch failed: %v", r.Name, r.Err)
	case r.Sync.Behind > 0 && r.Sync.Ahead > 0:
		return fmt.Sprintf("[%s] fetched · %d behind, %d ahead", r.Name, r.Sync.Behind, r.Sync.Ahead)
	case r.Sync.Behind > 0:
		return fmt.Sprintf("[%s] fetched · %d behind", r.Name, r.Sync.Behind)
	case r.Sync.Ahead > 0:
		return fmt.Sprintf("[%s] fetched · %d ahead", r.Name, r.Sync.Ahead)
	default:
		return fmt.Sprintf("[%s] fetched · up to date", r.Name)
	}
}

// fetchSummary is the status line for a finished fetch-all: how many checkouts were
// fetched and what it turned up. failed reports whether anything errored, which the caller
// uses to decide whether to force the log pane open (the per-repo reason is only in there).
func fetchSummary(results []addon.FetchResult) (line string, failed bool) {
	behind, failedN := 0, 0
	for _, r := range results {
		if r.Err != nil {
			failedN++
			continue
		}
		if r.Sync.Behind > 0 {
			behind++
		}
	}
	line = fmt.Sprintf("fetched %d git checkout(s)", len(results)-failedN)
	if behind > 0 {
		line += fmt.Sprintf(" · %d behind origin", behind)
	}
	if failedN > 0 {
		line += fmt.Sprintf(" · %d failed", failedN)
	}
	return line, failedN > 0
}

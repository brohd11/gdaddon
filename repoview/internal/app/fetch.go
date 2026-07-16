package app

import (
	"context"
	"fmt"
	"time"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"

	tea "github.com/charmbracelet/bubbletea"
)

// fetchDone carries a fetch-all pass's results back to the root via a broadcast, where they're
// logged and the list is rebuilt from the freshly updated refs.
type fetchDone struct {
	results []repo.FetchResult
}

// fetchTimeout caps the whole fan-out so an unreachable remote can't leave the fetch pending
// (and the root stuck marked as fetching) forever.
const fetchTimeout = 90 * time.Second

// fetchAllCmd runs `git fetch` concurrently in every scanned repo, off the UI thread. The repo
// set is captured up front and nothing touches Shared inside the goroutine; the results ride
// back on the broadcast. Every scanned repo is a git checkout (repo.Scan only returns those),
// so the whole set is passed straight to repo.FetchAll.
func fetchAllCmd(sh *core.Shared) tea.Cmd {
	repos := Of(sh).Repos
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		return core.PropagateAll(fetchDone{results: repo.FetchAll(ctx, repos)})
	}
}

// fetchLine describes one repo's fetch outcome for the output log.
func fetchLine(r repo.FetchResult) string {
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

// fetchSummary is the status line for a finished fetch-all: how many repos were fetched and
// what it turned up. failed reports whether anything errored (the per-repo reason is in the log).
func fetchSummary(results []repo.FetchResult) (line string, failed bool) {
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
	line = fmt.Sprintf("fetched %d repo(s)", len(results)-failedN)
	if behind > 0 {
		line += fmt.Sprintf(" · %d behind origin", behind)
	}
	if failedN > 0 {
		line += fmt.Sprintf(" · %d failed", failedN)
	}
	return line, failedN > 0
}

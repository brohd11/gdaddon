package tui

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/source"
)

// ---------- messages ----------

// releasesMsg / branchesMsg carry the result of an upstream fetch back to the
// loading screen.
type releasesMsg struct {
	listing *source.Listing
	err     error
}

type branchesMsg struct {
	branches []source.Asset
	err      error
}

// installEvent streams a streaming task's progress (one line per event) and its
// terminating done event (with any error/result payload).
type installEvent struct {
	line    string
	done    bool
	err     error
	path    string // resolved install path (single-install done event)
	version string // version read from the installed plugin.cfg
}

// installDoneMsg / installAllDoneMsg / pluginAddedMsg are routed by the router to
// the browse (root) screen: they refresh its list and set the status line, then
// the router unwinds back to it. reloadAddonsMsg rebuilds the list when rows were
// added (import / new plugin); applyStatusesMsg updates existing rows in place.
type installDoneMsg struct {
	statuses []addon.Status
	name     string
	version  string
}

type installAllDoneMsg struct {
	statuses []addon.Status
}

type reloadAddonsMsg struct {
	status   string
	statuses []addon.Status
}

// archiveFinishedMsg unwinds to the versions screen and re-lists it so the newly
// archived packages appear.
type archiveFinishedMsg struct{}

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

// msgRootRefresh is routed by the router to the browse (root) screen: it sets the
// status line, refreshes the list, then the router unwinds back to the root. The
// sender supplies the display text; rebuild picks setItems (row count changed, e.g.
// import / new plugin) over applyStatuses (existing rows updated in place).
type msgRootRefresh struct {
	status   string         // sender-provided display text
	statuses []addon.Status // nil ⇒ no refresh (error paths send nil)
	rebuild  bool           // true ⇒ setItems, false ⇒ applyStatuses
}

// archiveFinishedMsg unwinds to the versions screen and re-lists it so the newly
// archived packages appear.
type archiveFinishedMsg struct{}

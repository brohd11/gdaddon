package core

import "gdaddon/internal/addon"

// ---------- messages ----------

// InstallEvent streams a streaming task's progress (one line per event) and its
// terminating done event (with any error/result payload). Produced by domain task
// commands, consumed by the task screen; fields are exported for cross-package use.
type InstallEvent struct {
	Line    string
	Done    bool
	Err     error
	Path    string // resolved install path (single-install done event)
	Version string // version read from the installed plugin.cfg
}

// MsgRootRefresh is routed by the router to the browse (root) screen: it sets the
// status line, refreshes the list, then the router unwinds back to the root. The
// sender supplies the display text; Rebuild picks setItems (row count changed, e.g.
// import / new plugin) over applyStatuses (existing rows updated in place).
type MsgRootRefresh struct {
	Status   string         // sender-provided display text
	Statuses []addon.Status // nil ⇒ no refresh (error paths send nil)
	Rebuild  bool           // true ⇒ setItems, false ⇒ applyStatuses
}

// ArchiveFinishedMsg unwinds to the versions screen and re-lists it so the newly
// archived packages appear.
type ArchiveFinishedMsg struct{}

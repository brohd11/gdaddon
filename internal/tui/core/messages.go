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

// MsgGlobalRefresh asks the router to show the Global tab's root rebuilt: it finds
// the tab whose root handles this (the global list), switches to it, unwinds to its
// root, and that root reloads itself from disk. Sibling to MsgRootRefresh (browse),
// but routed by handler rather than a fixed tab index so it works from any tab —
// global Remove and New Plugin → Global both use it.
type MsgGlobalRefresh struct{ Status string }

// MsgArchiveRefresh asks the router to show the Archive tab's root rebuilt after a
// change to the local archive (a package removal). Routed by handler like
// MsgGlobalRefresh, so the deep remove flow can refresh the tab from anywhere.
type MsgArchiveRefresh struct{ Status string }

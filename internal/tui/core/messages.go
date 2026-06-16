package core

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

// RefreshTarget names which tab's root a MsgRefresh is meant for. The router stays
// tab-agnostic — it doesn't map a target to an index itself; each root claims the
// target(s) it handles in HandleRoot.
type RefreshTarget int

const (
	RefreshProject RefreshTarget = iota // the browse/project addon list
	RefreshGlobal                       // the global plugin list
	RefreshArchive                      // the local package archive
)

// MsgRefresh asks the router to refresh a tab's root after an out-of-band change.
// The router finds the root that claims Target, hands it the message to rebuild
// itself from disk, and — when Switch is set — makes that tab active and unwinds it
// to its root. One message serves every tab; it carries no list state, since each
// root reloads from the manifest/list/archive itself.
type MsgRefresh struct {
	Target RefreshTarget // which tab root handles this
	Switch bool          // true ⇒ switch to + unwind that tab; false ⇒ refresh in place
	Status string        // sender-provided display text
}

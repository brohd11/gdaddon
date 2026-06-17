package core

// ---------- messages ----------

// TaskEvent streams a streaming task's progress (one line per event) and its
// terminating done event (with any error and an opaque result Payload). Produced by
// a consumer's task command, consumed by the task screen; the framework only routes
// it, so Payload carries whatever the consumer's onDone needs (recover it with a
// type assertion). Fields are exported for cross-package use.
type TaskEvent struct {
	Line    string
	Done    bool
	Err     error
	Payload any // consumer-defined result for the terminating (Done) event
}

// MsgRefresh asks the router to refresh a tab's root after an out-of-band change.
// The router finds the root that claims Target, hands it the message to rebuild
// itself, and — when Switch is set — makes that tab active and unwinds it to its
// root. One message serves every tab; it carries no list state, since each root
// reloads itself. The router never interprets Target (it's opaque any): each root
// compares it against its own consumer-defined identifier in HandleRoot.
type MsgRefresh struct {
	Target any    // which tab root handles this (consumer-defined identifier)
	Switch bool   // true ⇒ switch to + unwind that tab; false ⇒ refresh in place
	Status string // sender-provided display text
}

// MsgThemeChanged tells the router the active theme changed (after SetTheme), so it
// rebuilds every cached tab root from its constructor to pick up the new palette.
// Raised by ApplyTheme; the router handles it centrally, so no screen interprets it.
type MsgThemeChanged struct{}

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

// propagateMsg carries an opaque payload the router broadcasts to every Receiver
// (PropagateAll). The router never interprets the payload — each screen type-switches
// on payloads it recognizes — so no new router case is needed per notification kind.
type propagateMsg struct{ payload any }

// MsgThemeChanged tells the router the active theme changed (after SetTheme), so it
// rebuilds every cached tab root from its constructor to pick up the new palette.
// Raised by ApplyTheme; the router handles it centrally, so no screen interprets it.
type MsgThemeChanged struct{}

package core

// ---------- messages ----------

// ctrlMsg marks the framework's control messages — the ones the router applies to the
// navigation stack synchronously (rather than dispatching to the active screen). Screens
// return these in the control-message lane of Update; the marker lets the router also
// recognize them when they arrive via the queue (an async cmd's result, Init, a batch).
type ctrlMsg interface{ isCtrl() }

func (propagateMsg) isCtrl()   {}
func (statusSetMsg) isCtrl()   {}
func (statusClearMsg) isCtrl() {}

// statusSetMsg used to set the status, will automatically append a timer to the outgoing cmds
type statusSetMsg struct {
	str       string
	wrLog     bool
	forceShow bool
}

func SetStatus(line string) Action {
	return Action{Msg: statusSetMsg{str: line}}
}

func SetStatusAndLog(line string, forceShow ...bool) Action {
	shw := GetOptional(false, forceShow...)
	return Action{Msg: statusSetMsg{str: line, wrLog: true, forceShow: shw}}
}

// statusClearMsg is the router's auto-clear timer firing for the status line: a tick
// scheduled when the status's generation advances (a fresh write). It clears the
// status only if gen still matches the current generation, so a newer write (which
// bumped the generation) leaves the stale tick a no-op. Resolved on the control path
// like the other ctrlMsgs (see router.applyCtrl).
type statusClearMsg struct{ gen int }

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

// MsgThemeChanged is the broadcast payload announcing the active theme changed (after
// SetTheme). ApplyTheme raises it via PropagateAll, so the router only routes it — it's
// not a control message. A consumer's App (or any Receiver) recognizes it and typically
// returns RefreshRoots() to rebuild the cached tab roots with the new palette, so the
// consumer owns that policy rather than the framework hard-coding it.
type MsgThemeChanged struct{}

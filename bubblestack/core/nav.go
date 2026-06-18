package core

// Navigation messages. Screens never mutate the stack directly; they return one of
// these (wrapped in an Action by the constructors below) as the control-message lane
// of Update and the router interprets it in one place, synchronously (same tick). This
// keeps screens decoupled and lets async handlers navigate too: an async cmd can return
// an Action as its result message, and the router resolves it when it arrives (see the
// router's Update / resolveCtrl).
type (
	pushMsg        struct{ s Screen }      // push a new screen on top
	popMsg         struct{ n int }         // pop n screens (back / cancel)
	popToMsg       struct{}                // pop to the nearest PopStopper, or the root
	replaceMsg     struct{ s Screen }      // pop current + push (e.g. fetching -> versions)
	resetToRootMsg struct{}                // unwind to the root (browse) screen
	showTabMsg     struct{ title string }  // make the tab with this title active, at its root
	seqMsg         struct{ acts []Action } // a sequence of Actions applied in order
)

func (pushMsg) isCtrl()        {}
func (popMsg) isCtrl()         {}
func (popToMsg) isCtrl()       {}
func (replaceMsg) isCtrl()     {}
func (resetToRootMsg) isCtrl() {}
func (showTabMsg) isCtrl()     {}
func (seqMsg) isCtrl()         {}

// The nav constructors each return an Action whose control-message lane (Msg) carries
// the navigation; the async lane (Cmd) is left nil. A screen returns one directly from
// Update, or pairs it with an async cmd via a struct literal.

func Push(s Screen) Action { return Action{Msg: pushMsg{s}} }

// Pop pops one screen by default, or n when given. Variadic so existing Pop()
// callers are unchanged; Pop(2) pops two levels (a sub-flow returning past its
// own intermediate screens). The router clamps so the root is never popped.
func Pop(n ...int) Action {
	count := GetOptional(1, n...)
	return Action{Msg: popMsg{count}}
}

// PopTo unwinds to the nearest screen that opts into PopStopper (a command hub),
// or the root if none — so a deep sub-flow can return to its hub without knowing
// the stack depth.
func PopTo() Action { return Action{Msg: popToMsg{}} }

func Replace(s Screen) Action { return Action{Msg: replaceMsg{s}} }
func ResetToRoot() Action     { return Action{Msg: resetToRootMsg{}} }

// PropagateAll broadcasts payload to every tab root and the active stack's screens.
// Each Receiver reacts to payloads it recognizes; the framework never interprets the
// payload (it's opaque any), so no router case is added per notification kind. Works
// from any tab — e.g. a refresh after an out-of-band change, where each root reloads
// itself and (optionally) returns a ShowTab Action to grab focus.
func PropagateAll(payload any) Action { return Action{Msg: propagateMsg{payload}} }

// ShowTab makes the tab whose Title == title active and unwinds its stack to its root.
// A no-op when no tab matches. Lets a reacting screen (or a menu) jump to a tab by name
// without the router knowing any tab identity beyond the title it already renders.
func ShowTab(title string) Action { return Action{Msg: showTabMsg{title}} }

// Seq groups Actions so a screen can issue several from one Update return — the
// synchronous sibling of tea.Batch (which batches async cmds, not control). The router
// applies them in order, in the same tick; zero (empty) Actions are skipped. Use it when
// one action needs more than one nav effect, e.g. reload a tab and pop the submenu.
func Seq(acts ...Action) Action { return Action{Msg: seqMsg{acts}} }

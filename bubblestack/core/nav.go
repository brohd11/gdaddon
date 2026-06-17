package core

import tea "github.com/charmbracelet/bubbletea"

// Navigation messages. Screens never mutate the stack directly; they return one
// of these via a command and the router interprets it in one place. This keeps
// screens decoupled and lets async handlers (a fetch result, a task event)
// navigate the same way.
type (
	pushMsg        struct{ s Screen }     // push a new screen on top
	popMsg         struct{ n int }        // pop n screens (back / cancel)
	popToMsg       struct{}               // pop to the nearest PopStopper, or the root
	replaceMsg     struct{ s Screen }     // pop current + push (e.g. fetching -> versions)
	resetToRootMsg struct{}               // unwind to the root (browse) screen
	showTabMsg     struct{ title string } // make the tab with this title active, at its root
)

func Push(s Screen) tea.Cmd { return func() tea.Msg { return pushMsg{s} } }

// Pop pops one screen by default, or n when given. Variadic so existing Pop()
// callers are unchanged; Pop(2) pops two levels (a sub-flow returning past its
// own intermediate screens). The router clamps so the root is never popped.
func Pop(n ...int) tea.Cmd {
	count := 1
	if len(n) > 0 {
		count = n[0]
	}
	return func() tea.Msg { return popMsg{count} }
}

// PopTo unwinds to the nearest screen that opts into PopStopper (a command hub),
// or the root if none — so a deep sub-flow can return to its hub without knowing
// the stack depth.
func PopTo() tea.Cmd { return func() tea.Msg { return popToMsg{} } }

func Replace(s Screen) tea.Cmd { return func() tea.Msg { return replaceMsg{s} } }
func ResetToRoot() tea.Cmd     { return func() tea.Msg { return resetToRootMsg{} } }

// PropagateAll broadcasts payload to every tab root and the active stack's screens.
// Each Receiver reacts to payloads it recognizes; the framework never interprets the
// payload (it's opaque any), so no router case is added per notification kind. Works
// from any tab — e.g. a refresh after an out-of-band change, where each root reloads
// itself and (optionally) returns a ShowTab command to grab focus.
func PropagateAll(payload any) tea.Cmd {
	return func() tea.Msg { return propagateMsg{payload} }
}

// ShowTab makes the tab whose Title == title active and unwinds its stack to its root.
// A no-op when no tab matches. Lets a reacting screen (or a menu) jump to a tab by name
// without the router knowing any tab identity beyond the title it already renders.
func ShowTab(title string) tea.Cmd {
	return func() tea.Msg { return showTabMsg{title} }
}

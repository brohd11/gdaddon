package core

import (
	"gdaddon/internal/addon"

	tea "github.com/charmbracelet/bubbletea"
)

// Navigation messages. Screens never mutate the stack directly; they return one
// of these via a command and the router interprets it in one place. This keeps
// screens decoupled and lets async handlers (a fetch result, a task event)
// navigate the same way.
type (
	pushMsg        struct{ s Screen } // push a new screen on top
	popMsg         struct{ n int }    // pop n screens (back / cancel)
	popToMsg       struct{}           // pop to the nearest PopStopper, or the root
	replaceMsg     struct{ s Screen } // pop current + push (e.g. fetching -> versions)
	resetToRootMsg struct{}           // unwind to the root (browse) screen
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

// rootRefresh asks the router to unwind to the root and refresh the browse list
// with the given status text and statuses (rebuild ⇒ row count changed).
func RootRefresh(status string, ss []addon.Status, rebuild bool) tea.Cmd {
	return func() tea.Msg { return MsgRootRefresh{status, ss, rebuild} }
}

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
	popMsg         struct{}           // pop the current screen (back / cancel)
	replaceMsg     struct{ s Screen } // pop current + push (e.g. fetching -> versions)
	resetToRootMsg struct{}           // unwind to the root (browse) screen
)

func Push(s Screen) tea.Cmd    { return func() tea.Msg { return pushMsg{s} } }
func Pop() tea.Cmd             { return func() tea.Msg { return popMsg{} } }
func Replace(s Screen) tea.Cmd { return func() tea.Msg { return replaceMsg{s} } }
func ResetToRoot() tea.Cmd     { return func() tea.Msg { return resetToRootMsg{} } }

// rootRefresh asks the router to unwind to the root and refresh the browse list
// with the given status text and statuses (rebuild ⇒ row count changed).
func RootRefresh(status string, ss []addon.Status, rebuild bool) tea.Cmd {
	return func() tea.Msg { return MsgRootRefresh{status, ss, rebuild} }
}

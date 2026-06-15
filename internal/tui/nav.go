package tui

import (
	"gdaddon/internal/addon"

	tea "github.com/charmbracelet/bubbletea"
)

// Navigation messages. Screens never mutate the stack directly; they return one
// of these via a command and the router interprets it in one place. This keeps
// screens decoupled and lets async handlers (a fetch result, a task event)
// navigate the same way.
type (
	pushMsg        struct{ s screen } // push a new screen on top
	popMsg         struct{}           // pop the current screen (back / cancel)
	replaceMsg     struct{ s screen } // pop current + push (e.g. fetching -> versions)
	resetToRootMsg struct{}           // unwind to the root (browse) screen
)

func push(s screen) tea.Cmd    { return func() tea.Msg { return pushMsg{s} } }
func pop() tea.Cmd             { return func() tea.Msg { return popMsg{} } }
func replace(s screen) tea.Cmd { return func() tea.Msg { return replaceMsg{s} } }
func resetToRoot() tea.Cmd     { return func() tea.Msg { return resetToRootMsg{} } }

// rootRefresh asks the router to unwind to the root and refresh the browse list
// with the given status text and statuses (rebuild ⇒ row count changed).
func rootRefresh(status string, ss []addon.Status, rebuild bool) tea.Cmd {
	return func() tea.Msg { return msgRootRefresh{status, ss, rebuild} }
}

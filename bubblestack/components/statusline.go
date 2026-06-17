package components

import (
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/lipgloss"
)

// StatusLine is the default core.Status: a transient, themed one-liner the router draws
// below the body and auto-clears ~5s after the last write (the router schedules the
// timer). The generation counter — bumped on Set, read via Gen — lets a newer write
// override a pending clear, so a stale timer never wipes a fresh message. Supply it via
// bubblestack.Config.Status, or implement core.Status for a custom element. It is
// context-agnostic — it names no domain type.
type StatusLine struct {
	msg string
	gen int
}

// NewStatusLine builds the default status line.
func NewStatusLine() *StatusLine { return &StatusLine{} }

// Set replaces the message and bumps the generation; Set("") clears it (Shown() is
// then false). The bump is what makes the most recent write own the auto-clear window.
func (s *StatusLine) Set(line string) { s.msg = line; s.gen++ }

// Clear drops the message without bumping the generation, so a pending tick for the
// just-cleared generation simply finds nothing to do.
func (s *StatusLine) Clear() { s.msg = "" }

func (s *StatusLine) Shown() bool { return s.msg != "" }
func (s *StatusLine) Gen() int    { return s.gen }

func (s *StatusLine) Height() int {
	if s.msg == "" {
		return 0
	}
	return lipgloss.Height(s.View())
}

// View renders the message in the themed status style, read at render time so a theme
// switch repaints it. Empty when there's no message.
func (s *StatusLine) View() string {
	if s.msg == "" {
		return ""
	}
	return core.StatusStyle().Render(s.msg)
}

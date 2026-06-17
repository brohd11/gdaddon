package core

import tea "github.com/charmbracelet/bubbletea"

// screen is one navigable view. The router owns the optional chrome (header, status,
// output pane — see chrome.go), the help bar, and the navigation stack; a screen
// renders only its own body and handles its own keys. Implementations are pointer
// types so Update and SetSize can mutate in place.
// Update returns the (possibly new) screen, a synchronous control message the router
// applies inline this same tick (a nav command from core; nil for none), and a genuine
// async tea.Cmd the router hands to bubbletea (IO/streaming; nil for none). Splitting the
// two lanes means navigation no longer round-trips through the command queue, while real
// IO can only travel in the cmd lane (so nothing can accidentally block the update loop).
type Screen interface {
	Init(*Shared) tea.Cmd
	Update(*Shared, tea.Msg) (Screen, tea.Msg, tea.Cmd)
	View(*Shared) string     // body between the header and the help bar
	HelpView(*Shared) string // the fully-rendered help bar line(s)
	SetSize(s *Shared, width, bodyHeight int)
}

// Optional behaviors the router type-asserts for, so a screen only opts in when
// relevant rather than every screen carrying a stub.

// filterer reports an active text filter, so the router's global single-key
// shortcuts (tab/c) don't steal keystrokes meant for the filter input.
type Filterer interface{ Filtering() bool }

// receiver lets a screen react to a broadcast notification (PropagateAll). The
// framework only routes the payload (opaque, consumer-defined); a screen type-
// switches on payloads it recognizes and ignores the rest. It may return a control
// message (e.g. ShowTab to grab focus), which the router resolves in the same tick, or
// nil. Optional — screens opt in by implementing it.
type Receiver interface {
	Receive(sh *Shared, payload any) tea.Msg
}

// popStopper marks a screen the router stops at when handling PopTo: a sub-flow
// can pop back to its command hub (the nearest stopper) without knowing the stack
// depth. Returns false to act as a normal screen.
type PopStopper interface{ PopStop() bool }

// ChromeMask marks which chrome elements a screen suppresses while it is on top
// (true ⇒ hidden). The zero value hides nothing; FullscreenMask hides everything,
// giving a screen the whole canvas.
type ChromeMask struct {
	Header   bool
	TabStrip bool
	Status   bool
	Output   bool
	Help     bool
}

// FullscreenMask suppresses every chrome element — the mask a fullscreen screen
// returns from ChromeMask.
func FullscreenMask() ChromeMask {
	return ChromeMask{Header: true, TabStrip: true, Status: true, Output: true, Help: true}
}

// chromeMasker is the optional interface a screen implements to suppress chrome
// elements while it is the active (top) screen. The router queries the top screen
// each render and resize, so popping back to a screen that doesn't implement it
// restores the chrome automatically — no shared state to reset.
type ChromeMasker interface{ ChromeMask() ChromeMask }

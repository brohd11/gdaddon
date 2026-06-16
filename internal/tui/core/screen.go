package core

import tea "github.com/charmbracelet/bubbletea"

// screen is one navigable view. The router owns the shared chrome (header, help
// bar, output pane) and the navigation stack; a screen renders only its own body
// and handles its own keys. Implementations are pointer types so Update and
// SetSize can mutate in place.
type Screen interface {
	Init(*Shared) tea.Cmd
	Update(*Shared, tea.Msg) (Screen, tea.Cmd)
	View(*Shared) string     // body between the header and the help bar
	HelpView(*Shared) string // the fully-rendered help bar line(s)
	SetSize(s *Shared, width, bodyHeight int)
}

// Optional behaviors the router type-asserts for, so a screen only opts in when
// relevant rather than every screen carrying a stub.

// filterer reports an active text filter, so the router's global single-key
// shortcuts (tab/c) don't steal keystrokes meant for the filter input.
type Filterer interface{ Filtering() bool }

// outputViewer reports that the screen shows the shared output/log pane below
// it (browse + the task screens), used for sizing and for whether tab focuses
// the output.
type OutputViewer interface{ WantsOutput() bool }

// rootHandler lets a tab's root screen handle app-level result messages itself,
// so the router stays tab-agnostic (it only owns the stack). browse uses this to
// refresh its addon list. Returns whether the message was consumed.
type RootHandler interface {
	HandleRoot(sh *Shared, msg tea.Msg) (handled bool)
}

// relister is a screen that can re-list itself after a background task changed its
// data (the versions screen after an archive). The router unwinds to it and calls
// relist without naming the concrete type, so it can live in another package.
type Relister interface{ Relist() }

package tui

import tea "github.com/charmbracelet/bubbletea"

// screen is one navigable view. The router owns the shared chrome (header, help
// bar, output pane) and the navigation stack; a screen renders only its own body
// and handles its own keys. Implementations are pointer types so Update and
// SetSize can mutate in place.
type screen interface {
	Init(*shared) tea.Cmd
	Update(*shared, tea.Msg) (screen, tea.Cmd)
	View(*shared) string     // body between the header and the help bar
	HelpView(*shared) string // the fully-rendered help bar line(s)
	SetSize(s *shared, width, bodyHeight int)
}

// Optional behaviors the router type-asserts for, so a screen only opts in when
// relevant rather than every screen carrying a stub.

// filterer reports an active text filter, so the router's global single-key
// shortcuts (tab/c) don't steal keystrokes meant for the filter input.
type filterer interface{ filtering() bool }

// outputViewer reports that the screen shows the shared output/log pane below
// it (browse + the task screens), used for sizing and for whether tab focuses
// the output.
type outputViewer interface{ wantsOutput() bool }

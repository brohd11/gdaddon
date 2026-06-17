package core

import tea "github.com/charmbracelet/bubbletea"

// Chrome is the optional UI furniture the router draws around the active screen's
// body: a persistent header box, a transient status line, and a pluggable output
// pane. Each element is independently optional — a nil Header/Output (or empty
// Status) is simply not drawn — and independently toggleable at runtime
// (sh.Chrome.Header.Hide(), sh.Chrome.Output.Toggle()). A screen can also suppress
// any element while it is on top via ChromeMasker (see screen.go). Shared.Chrome is
// nil for a fullscreen-by-default app, in which case the router renders only the
// body + help bar.
//
// The framework names no domain type here: Header is a consumer closure and Output
// is an interface (its default implementation, the scrollable log pane, lives in
// components — core ← components, so core only names the interface).
type Chrome struct {
	Header *HeaderPane // nil ⇒ no header box
	Output Output      // nil ⇒ no output pane (default impl: components.LogPane)
	Status Status      // nil ⇒ no status line (default impl: components.StatusLine)

	// outputFocused routes input to the output pane (scrolling) instead of the
	// active screen. Owned by the router; the pane itself is focus-agnostic and only
	// renders a focused affordance from the bool passed to View.
	outputFocused bool
}

// HeaderPane wraps a consumer's context-box renderer with a runtime hidden flag, so
// the header can be toggled off without dropping the closure.
type HeaderPane struct {
	Render func(*Shared) string
	hidden bool
}

// NewHeaderPane wraps a header renderer (the closure a consumer supplies). The
// facade's Run builds this from Config.Header.
func NewHeaderPane(render func(*Shared) string) *HeaderPane { return &HeaderPane{Render: render} }

func (h *HeaderPane) Hide()        { h.hidden = true }
func (h *HeaderPane) Show()        { h.hidden = false }
func (h *HeaderPane) Toggle()      { h.hidden = !h.hidden }
func (h *HeaderPane) Hidden() bool { return h.hidden }

// view renders the header, or "" when the pane is nil, hidden, or has no renderer —
// so the router measures/draws it uniformly. Nil-receiver safe.
func (h *HeaderPane) view(s *Shared) string {
	if h == nil || h.hidden || h.Render == nil {
		return ""
	}
	return h.Render(s)
}

// Output is a pluggable below-body pane the router renders, sizes, and — while it
// holds focus — feeds scroll keys. The default implementation is the scrollable log
// in components (NewLogPane); a consumer may supply its own. The router treats it
// opaquely: logging is a separate capability (Log(string)) discovered by Shared.Log
// via type assertion, so an Output need not be a log.
type Output interface {
	Shown() bool                       // occupies layout space when true
	Toggle()                           // show/hide (the Output key, `o`)
	Hide()                             // collapse (e.g. focus returning to the body)
	Clear()                            // drop contents (the Clear key)
	SetSize(termWidth, termHeight int) // lay out to the terminal; the pane picks its own height
	Height() int                       // rows occupied when shown (0 when hidden)
	View(focused bool) string          // render (focused ⇒ scroll affordance)
	Update(msg tea.Msg) tea.Cmd        // handle a key while focused (scrolling)
	GotoBottom()                       // pin to the newest content
}

// Status is the pluggable transient one-liner the router draws below the body
// (parallel to Output). The default implementation is the themed line in components
// (NewStatusLine); a consumer may supply its own. Core treats it opaquely: it Sets the
// text (via Shared.WriteStatus/SetStatus), measures it (Shown/Height), renders it
// (View), and clears it — explicitly (the Clear key) or via the auto-clear timer the
// router schedules and keys on Gen, so a newer write's timer never lets a stale one
// clear a fresh message.
type Status interface {
	Set(line string) // replace the message and bump the generation
	Clear()          // drop the message (does NOT bump the generation)
	Shown() bool     // occupies layout space (non-empty message)
	Height() int     // rows occupied when shown (0 when empty)
	View() string    // render the themed line
	Gen() int        // current generation; the auto-clear timer compares against this
}

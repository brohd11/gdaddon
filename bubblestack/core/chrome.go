package core

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	Header     *HeaderPane     // nil ⇒ no header box
	Breadcrumb *BreadcrumbPane // nil ⇒ breadcrumb still drawn (default, shown); set to toggle it
	Output     Output          // nil ⇒ no output pane (default impl: components.LogPane)
	Status     Status          // nil ⇒ no status line (default impl: components.StatusLine)

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

// BreadcrumbPane carries the runtime hidden flag for the router-drawn breadcrumb
// bar (built from the live nav stack — see Router.breadcrumbView), so the breadcrumb
// can be toggled off without the router tracking visibility itself. Parallel to
// HeaderPane.
type BreadcrumbPane struct {
	hidden bool
}

// NewBreadcrumbPane returns a shown breadcrumb pane. The facade's Run sets this on
// Chrome so a consumer can sh.Chrome.Breadcrumb.Hide() it.
func NewBreadcrumbPane() *BreadcrumbPane { return &BreadcrumbPane{} }

func (b *BreadcrumbPane) Hide()        { b.hidden = true }
func (b *BreadcrumbPane) Show()        { b.hidden = false }
func (b *BreadcrumbPane) Toggle()      { b.hidden = !b.hidden }
func (b *BreadcrumbPane) Hidden() bool { return b.hidden }

// view renders the breadcrumb bar plus a full-width rule under it (mirroring the tab
// strip), or "" when the pane is hidden or there are no crumbs. A nil pane renders
// normally (shown), so the router can hand crumbs to it uniformly. Nil-receiver safe.
func (b *BreadcrumbPane) view(crumbs []Crumb, width int) string {
	if b != nil && b.hidden {
		return ""
	}
	bar := RenderBreadcrumb(crumbs, width)
	if bar == "" || width <= 0 {
		return bar
	}
	rule := breadcrumbRuleStyle.Render(strings.Repeat("─", width))
	return lipgloss.JoinVertical(lipgloss.Left, bar, rule)
}

// Crumb is one segment of the router-drawn breadcrumb: a full label and an optional
// shorter form used when the trail is too wide. Short falls back to Full when empty.
type Crumb struct {
	Full  string
	Short string
}

func (c Crumb) pick(short bool) string {
	if short && c.Short != "" {
		return c.Short
	}
	return c.Full
}

// crumbSep separates breadcrumb segments.
const crumbSep = " › "

// RenderBreadcrumb joins crumbs into the styled breadcrumb bar the router draws under
// the tab strip: upstream segments + separators muted, the current (last) segment in
// the accent. When the full trail is too wide for width it retries with the short form
// of every segment but the last, then left-truncates the whole bar — keeping the
// current segment visible. An empty slice renders nothing.
func RenderBreadcrumb(crumbs []Crumb, width int) string {
	if len(crumbs) == 0 {
		return ""
	}
	last := len(crumbs) - 1
	labels := func(short bool) []string {
		out := make([]string, len(crumbs))
		for i, c := range crumbs {
			out[i] = c.pick(short && i != last)
		}
		return out
	}
	avail := width - 2 // breadcrumbBarStyle's horizontal padding
	chosen := labels(false)
	if width > 0 && lipgloss.Width(strings.Join(chosen, crumbSep)) > avail {
		chosen = labels(true)
	}
	if width > 0 && lipgloss.Width(strings.Join(chosen, crumbSep)) > avail {
		// Last resort: truncate the whole trail, keeping the tail (current segment).
		return breadcrumbBarStyle.Render(crumbMutedStyle.Render(TruncLeft(strings.Join(chosen, crumbSep), avail)))
	}
	parts := make([]string, len(chosen))
	for i, l := range chosen {
		if i == last {
			parts[i] = crumbCurStyle.Render(l)
		} else {
			parts[i] = crumbMutedStyle.Render(l)
		}
	}
	return breadcrumbBarStyle.Render(strings.Join(parts, crumbMutedStyle.Render(crumbSep)))
}

// Output is a pluggable below-body pane the router renders, sizes, and — while it
// holds focus — feeds scroll keys. The default implementation is the scrollable log
// in components (NewLogPane); a consumer may supply its own. The router treats it
// opaquely: logging is a separate capability (Log(string)) discovered by Shared.Log
// via type assertion, so an Output need not be a log. Wrapping (Keys.Wrap) is
// likewise optional, discovered the same way — see Wrapper.
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
	Log(line string, forceShow bool)
}

// Wrapper is the optional second render mode an Output may offer: wrapping long lines
// rather than letting them truncate at the pane edge, which is otherwise the only fate
// of a line wider than the box (the viewport clips it, and there is no way to scroll
// to the tail). The router reaches it by type assertion on Keys.Wrap — the same
// optional-capability pattern as Shared.Log — so an Output need not implement it.
type Wrapper interface {
	ToggleWrap()
	Wrapped() bool
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

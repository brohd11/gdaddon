package core

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// maskOf is the chrome suppression requested by screen s, or the zero mask (hide
// nothing) when it doesn't implement ChromeMasker. Parameterized by screen (rather
// than always reading r.Top()) so the overlay path can frame the screen below the
// popup with that screen's own mask.
func (r Router) maskOf(s Screen) ChromeMask {
	if m, ok := s.(ChromeMasker); ok {
		return m.ChromeMask()
	}
	return ChromeMask{}
}

// currentMask is the mask of the active (top) screen.
func (r Router) currentMask() ChromeMask { return r.maskOf(r.Top()) }

// outputVisible reports whether an output pane currently occupies layout space
// (present and shown). It does not account for the per-screen mask.
func (r Router) outputVisible() bool {
	return r.sh.Chrome != nil && r.sh.Chrome.Output != nil && r.sh.Chrome.Output.Shown()
}

// helpViewFor is screen s's help bar, suppressed (empty) when its mask hides it.
// helpHeightFor measures it the same way so the body sizing stays in sync.
func (r Router) helpViewFor(s Screen, mask ChromeMask) string {
	if mask.Help {
		return ""
	}
	return s.HelpView(r.sh)
}

func (r Router) helpHeightFor(s Screen, mask ChromeMask) int {
	return vheight(r.helpViewFor(s, mask))
}

// tabStripView renders the top-level tab strip (omitted when there's only one
// tab): the tab titles followed by a full-width rule that delimits it from the
// content below.
func (r Router) tabStripView() string {
	if len(r.tabs) < 2 {
		return ""
	}
	tabs := make([]string, len(r.tabs))
	for i, t := range r.tabs {
		if i == r.active {
			tabs[i] = activeTabStyle.Render(t.Title)
		} else {
			tabs[i] = tabStyle.Render(t.Title)
		}
	}
	row := tabStripStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
	if r.sh.width <= 0 {
		return row
	}
	rule := tabRuleStyle.Render(strings.Repeat("─", r.sh.width))
	return lipgloss.JoinVertical(lipgloss.Left, row, rule)
}

// breadcrumbView builds the breadcrumb bar from the live nav stack: it asks each
// screen implementing Crumber for its segment (root→top, the top screen full and the
// upstream ones short), skips empty ones, and hands the crumbs to Chrome.Breadcrumb
// to render (joined path + separator rule, gated by the pane's hidden flag). Built
// fresh each frame so it always reflects the current stack — pushing/popping needs no
// breadcrumb bookkeeping.
func (r Router) breadcrumbView() string {
	var crumbs []Crumb
	for _, s := range r.stack {
		c, ok := s.(Crumber)
		if !ok {
			continue
		}
		full := c.CrumbLabel(false)
		if full == "" {
			continue
		}
		crumbs = append(crumbs, Crumb{Full: full, Short: c.CrumbLabel(true)})
	}
	var bc *BreadcrumbPane
	if r.sh.Chrome != nil {
		bc = r.sh.Chrome.Breadcrumb
	}
	return bc.view(crumbs, r.sh.width) // nil-safe: renders normally
}

// topChrome is the persistent chrome above the body: the header box, the tab strip
// (if any), and the breadcrumb bar below it, each gated by the active screen's mask.
// Its height is measured (not a constant) so adding/removing a part automatically
// reflows the body.
func (r Router) topChrome(mask ChromeMask) string {
	var parts []string
	if !mask.Header && r.sh.Chrome != nil {
		if header := r.sh.Chrome.Header.view(r.sh); header != "" { // nil-receiver safe
			parts = append(parts, header)
		}
	}
	if !mask.TabStrip {
		if strip := r.tabStripView(); strip != "" {
			parts = append(parts, strip)
		}
	}
	if !mask.Breadcrumb {
		if crumb := r.breadcrumbView(); crumb != "" {
			parts = append(parts, crumb)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// belowChrome is the chrome rendered between the active screen's body and the help
// bar: the status line (if any) then the output box (when shown), each gated by the
// screen's mask. Drawn by the router around every screen, so output/status persist
// across tab switches and screen pushes. Empty when there's neither.
func (r Router) belowChrome(mask ChromeMask) string {
	ch := r.sh.Chrome
	if ch == nil {
		return ""
	}
	var parts []string
	if !mask.Status && ch.Status != nil && ch.Status.Shown() {
		parts = append(parts, ch.Status.View())
	}
	if !mask.Output && r.outputVisible() {
		parts = append(parts, ch.Output.View(ch.outputFocused))
	}
	if len(parts) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// vheight is lipgloss.Height but reports 0 for an empty string (lipgloss.Height("")
// is 1), so optional chrome contributes no rows when absent.
func vheight(s string) int {
	if s == "" {
		return 0
	}
	return lipgloss.Height(s)
}

// bodyHeightFor is the rows available to screen s's body: the space between the top
// chrome and the help bar, minus the status/output chrome below the body.
func (r Router) bodyHeightFor(s Screen) int {
	mask := r.maskOf(s)
	h := r.sh.height - vheight(r.topChrome(mask)) - vheight(r.belowChrome(mask)) - r.helpHeightFor(s, mask)
	if h < 1 {
		h = 1
	}
	return h
}

func (r Router) resize() {
	if r.sh.width == 0 {
		return
	}
	// The output pane is router-owned chrome, so the router sizes it and keeps it
	// pinned to the newest line unless the user is scrolling it.
	if r.outputVisible() {
		r.sh.Chrome.Output.SetSize(r.sh.width, r.sh.height)
		if !r.sh.Chrome.outputFocused {
			r.sh.Chrome.Output.GotoBottom()
		}
	}
	// When an overlay (popup) is on top, the screen below it is still drawn as the
	// background, so it must be kept sized too — otherwise it goes stale on resize.
	if o, ok := r.Top().(Overlayer); ok && o.IsOverlay() && len(r.stack) >= 2 {
		below := r.stack[len(r.stack)-2]
		below.SetSize(r.sh, r.sh.width, r.bodyHeightFor(below))
	}
	r.Top().SetSize(r.sh, r.sh.width, r.bodyHeightFor(r.Top()))
}

// frame composes the persistent chrome (header/tab strip above, status/output and
// help below) around screen s's body — the full-screen layout the router shows for
// the active screen, and the background it draws a popup over (see View).
func (r Router) frame(s Screen) string {
	sh := r.sh
	mask := r.maskOf(s)
	chrome := r.topChrome(mask)
	body := s.View(sh)
	below := r.belowChrome(mask)
	help := r.helpViewFor(s, mask)
	// Pad the body so the status/output chrome and the always-visible help bar sit
	// at the very bottom.
	if pad := (sh.height - vheight(chrome) - vheight(below) - vheight(help)) - lipgloss.Height(body); pad > 0 {
		body = lipgloss.JoinVertical(lipgloss.Left, body, Blanks(pad))
	}
	var parts []string
	if chrome != "" {
		parts = append(parts, chrome)
	}
	parts = append(parts, body)
	if below != "" {
		parts = append(parts, below)
	}
	if help != "" {
		parts = append(parts, help)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (r Router) View() string {
	// An overlay (popup) on top draws the screen below it as the background, then
	// composites its own box centered over it — so the underlying screen stays
	// visible around the popup. Only the top screen receives input, so it's modal.
	if o, ok := r.Top().(Overlayer); ok && o.IsOverlay() && len(r.stack) >= 2 {
		bg := r.frame(r.stack[len(r.stack)-2])
		box := r.Top().View(r.sh)
		x := (r.sh.width - lipgloss.Width(box)) / 2
		y := (r.sh.height - lipgloss.Height(box)) / 2
		if y < 0 {
			y = 0
		}
		return Composite(bg, box, x, y)
	}
	return r.frame(r.Top())
}

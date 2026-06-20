package core

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// statusClearDelay is how long a status line stays up after the most recent write
// before the router's auto-clear timer hides it.
const statusClearDelay = 5 * time.Second

// tabEntry is one top-level tab: a title for the strip and a constructor for its
// root screen. The router builds the root lazily (NewRouter) and caches it, so the
// theme is already applied before the root bakes its styles; the same constructor
// is re-invoked to rebuild every root on a theme change. Roots reflect on-disk /
// manifest data, so reconstruction loses no per-tab state.
type TabEntry struct {
	Title string
	New   func(*Shared) Screen
}

// router is the top-level tea.Model. It owns the shared chrome state, the set of
// top-level tabs, and the active tab's navigation stack; it renders the tab strip
// + header + help bar (and the output pane) around the active screen, and
// translates navigation messages into stack operations. The active tab's root is
// the permanent bottom of the stack: a pop with a single screen left is ignored.
//
// Tabs are switched with [ / ] only at depth 1, so the live stack always belongs
// to the active tab; there is never a deeper stack to stash when switching.
type Router struct {
	sh     *Shared
	tabs   []TabEntry
	roots  []Screen // cached root per tab, built from tabs[i].New(sh)
	active int      // index into tabs of the visible tab
	stack  []Screen // live nav stack for the active tab; stack[0] == roots[active]

	// statusGen is the status generation the router has already scheduled an
	// auto-clear timer for; when the element's Gen advances past it (a fresh write),
	// scheduleStatusClear arms a new timer keyed on the new generation.
	statusGen int
}

func NewRouter(sh *Shared, tabs []TabEntry) Router {
	roots := make([]Screen, len(tabs))
	for i := range tabs {
		roots[i] = tabs[i].New(sh)
	}
	return Router{sh: sh, tabs: tabs, roots: roots, stack: []Screen{roots[0]}}
}

func (r Router) Top() Screen { return r.stack[len(r.stack)-1] }

func (r Router) Init() tea.Cmd { return r.Top().Init(r.sh) }

func (r Router) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Global keys are handled once, here, for whatever screen is on top: quit,
	// the output-pane focus/scroll mode, and tab/c (gated by the active screen's
	// filter so they don't steal filter keystrokes). globalKey returns an Action whose
	// control message is resolved inline and whose async cmd (e.g. tea.Quit) is queued.
	if key, ok := msg.(tea.KeyMsg); ok {
		if act, handled := r.globalKey(key); handled {
			r.apply(act, &cmds)
			r.resize()
			return r, tea.Batch(cmds...)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.sh.width, r.sh.height = msg.Width, msg.Height
		r.resize()
		return r, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		r.sh.Spinner, cmd = r.sh.Spinner.Update(msg)
		return r, cmd
	}

	// An Action that arrives via the queue (an async cmd returning a nav command, e.g.
	// finishInstallCmd → PropagateAll) is applied to the stack synchronously — the same
	// path as an Action a screen returns from Update.
	if act, ok := msg.(Action); ok {
		r.apply(act, &cmds)
		r.resize()
		return r, tea.Batch(cmds...)
	}
	// A bare control message arriving via the queue (a screen's Init, a batch, the
	// auto-clear tick) is resolved the same way.
	if _, ok := msg.(ctrlMsg); ok {
		r.resolveCtrl(msg, &cmds)
		r.resize()
		return r, tea.Batch(cmds...)
	}

	// Otherwise it's a screen message: dispatch to the active screen, then apply the
	// Action it returns — its control message inline (same tick) and its async cmd to
	// bubbletea.
	s, act := r.Top().Update(r.sh, msg)
	r.stack[len(r.stack)-1] = s
	r.apply(act, &cmds)

	// Re-lay-out after every message: cheap, and avoids chasing every spot that
	// changes content height (help expansion, log growth, screen switches).
	r.resize()
	return r, tea.Batch(cmds...)
}

// apply unpacks an Action: it resolves the control-message lane against the stack
// (synchronously, this same tick) and appends the async cmd lane to cmds for bubbletea.
func (r *Router) apply(act Action, cmds *[]tea.Cmd) {
	r.resolveCtrl(act.Msg, cmds)
	if act.Cmd != nil {
		*cmds = append(*cmds, act.Cmd)
	}
}

// resolveCtrl applies a control message (and any control messages it cascades into,
// e.g. a propagate whose Receivers each request a ShowTab) to the navigation stack in
// one tick, draining a worklist. Async cmds produced along the way (a pushed screen's
// Init) are collected into cmds for bubbletea. A nil message is a no-op.
func (r *Router) resolveCtrl(m tea.Msg, cmds *[]tea.Cmd) {
	queue := []tea.Msg{m}
	for len(queue) > 0 {
		m := queue[0]
		queue = queue[1:]
		if m == nil {
			continue
		}
		queue = append(queue, r.applyCtrl(m, cmds)...)
	}
}

// applyCtrl mutates the stack for one control message, returning any follow-up control
// messages (resolved next by resolveCtrl) and appending async cmds (a pushed/replaced
// screen's Init, or a Receiver's cmd lane) to cmds. A non-control message is ignored.
func (r *Router) applyCtrl(m tea.Msg, cmds *[]tea.Cmd) (follows []tea.Msg) {
	push := func(cmd tea.Cmd) {
		if cmd != nil {
			*cmds = append(*cmds, cmd)
		}
	}
	switch m := m.(type) {
	case pushMsg:
		r.stack = append(r.stack, m.s)
		push(m.s.Init(r.sh))

	case replaceMsg:
		r.stack[len(r.stack)-1] = m.s
		push(m.s.Init(r.sh))

	case popMsg:
		for i := 0; i < m.n && len(r.stack) > 1; i++ {
			r.stack = r.stack[:len(r.stack)-1]
		}

	case popToMsg:
		// Always leave the current screen, then stop at the first screen that opts
		// into PopStopper (a command hub), or the root.
		for len(r.stack) > 1 {
			r.stack = r.stack[:len(r.stack)-1]
			if s, ok := r.Top().(PopStopper); ok && s.PopStop() {
				break
			}
		}

	case resetToRootMsg:
		r.stack = r.stack[:1]

	case seqMsg:
		// Unpack each grouped Action: hand its control message back to resolveCtrl's
		// worklist (applied in order this same tick) and collect its async cmd.
		for _, a := range m.acts {
			if a.Msg != nil {
				follows = append(follows, a.Msg)
			}
			push(a.Cmd)
		}

	case showTabMsg:
		// Switch to the tab whose title matches and unwind it to its root. The router
		// addresses tabs only by the title it already renders — no separate identity.
		for i := range r.tabs {
			if r.tabs[i].Title == m.title {
				r.active = i
				r.stack = []Screen{r.roots[i]}
				break
			}
		}

	case propagateMsg:
		// Broadcast the opaque payload to every tab root, the active tab's deeper
		// screens, and the consumer's App — each Receiver claims what it recognizes.
		// The router never interprets the payload (no per-notification case). A Receiver
		// may return an Action (e.g. ShowTab to grab focus, or RefreshRoots after a theme
		// change), resolved in this same tick. The App is notified last so a follow-up it
		// returns lands after every live screen has seen the payload.
		notify := func(v any) {
			if rc, ok := v.(Receiver); ok {
				act := rc.Receive(r.sh, m.payload)
				if act.Msg != nil {
					follows = append(follows, act.Msg)
				}
				push(act.Cmd)
			}
		}
		for i := range r.roots {
			notify(r.roots[i])
		}
		for _, s := range r.stack[1:] { // the active root is already covered via r.roots[active]
			notify(s)
		}
		notify(r.sh.App)

	case refreshRootsMsg:
		// Rebuild every cached tab root from its constructor so each re-bakes its
		// delegate/list styles from the current palette (the consumer's reaction to a
		// theme change, via App.Receive → RefreshRoots). Deeper live screens are
		// transient (rebuilt on reopen) and the router-drawn chrome already repaints
		// from the refreshed style vars.
		for i := range r.roots {
			r.roots[i] = r.tabs[i].New(r.sh)
		}
		r.stack[0] = r.roots[r.active]

	case statusSetMsg:
		if m.str != "" { // empty messages don't print, so don't start timer
			r.sh.WriteStatus(m.str, m.wrLog, m.forceShow)
			statCmd := r.getStatusClear()
			if statCmd != nil {
				push(statCmd)
			}
		}
	case statusClearMsg:
		// The auto-clear timer fired: clear the status only if its generation hasn't
		// advanced since this tick was armed. A newer write bumped Gen past m.gen, so
		// a stale tick is a no-op (the fresh message keeps its own later timer).
		ch := r.sh.Chrome
		if ch != nil && ch.Status != nil && ch.Status.Gen() == m.gen {
			ch.Status.Clear()
		}
	}
	return follows
}

// Returns a timer command to reset the status line, called via setStatusMsg
func (r *Router) getStatusClear() tea.Cmd {
	ch := r.sh.Chrome
	if ch == nil || ch.Status == nil {
		return nil
	}
	g := ch.Status.Gen()
	if g == r.statusGen {
		return nil
	}
	r.statusGen = g
	if !ch.Status.Shown() {
		return nil
	}
	return tea.Tick(statusClearDelay, func(time.Time) tea.Msg {
		return statusClearMsg{gen: g}
	})
}

// globalKey handles the keys available in any screen. It returns (act, true) when it
// consumed the key — act carries a control message resolved inline and/or an async cmd
// (e.g. tea.Quit or an output-scroll cmd) — or (Action{}, false) to let the active
// screen handle it. Pointer receiver: [ / ] mutate active/stack, which must persist
// back to Update's router.
func (r *Router) globalKey(msg tea.KeyMsg) (Action, bool) {
	k := msg.String()
	if k == "ctrl+c" {
		return Async(tea.Quit), true
	}

	ch := r.sh.Chrome
	outputOn := ch != nil && ch.Output != nil

	// When the output pane holds focus, navigation keys scroll it; everything
	// else either toggles back or clears.
	if outputOn && ch.outputFocused {
		switch {
		case MatchKey(k, Keys.ToggleOutput), MatchKey(k, Keys.Back):
			ch.outputFocused = false
			return Action{}, true
		case MatchKey(k, Keys.Output):
			ch.Output.Hide()
			ch.outputFocused = false
			return Action{}, true
		case MatchKey(k, Keys.Clear):
			r.clearOutput()
			return Action{}, true
		case MatchKey(k, Keys.Quit):
			return Async(tea.Quit), true
		}
		return Async(ch.Output.Update(msg)), true
	}

	// tab jumps into the output pane, c clears the log, [ / ] switch top-level tabs
	// (only at the root, so the live stack always belongs to the active tab), and `
	// unwinds a deep stack back to the root for a quick exit — unless the active
	// screen is capturing filter text. The output keys pass through (no consume) when
	// there is no output pane, so a chromeless app can bind tab/o itself.
	if f, ok := r.Top().(Filterer); !ok || !f.Filtering() {
		switch {
		case MatchKey(k, Keys.ToggleOutput):
			if !outputOn {
				break
			}
			if ch.Output.Shown() {
				ch.outputFocused = true
				ch.Output.GotoBottom()
			}
			return Action{}, true
		case MatchKey(k, Keys.Output):
			if !outputOn {
				break
			}
			ch.Output.Toggle()
			if !ch.Output.Shown() {
				ch.outputFocused = false
			}
			return Action{}, true
		case MatchKey(k, Keys.Clear):
			if ch == nil {
				break
			}
			r.clearOutput()
			return Action{}, true
		case MatchKey(k, Keys.Quit):
			// q is the global quit, handled once here for every screen (the filter
			// gate above keeps it from firing while a list/form is capturing text).
			return Async(tea.Quit), true
		case MatchKey(k, Keys.NextTab):
			return Action{}, r.switchTab(1)
		case MatchKey(k, Keys.PrevTab):
			return Action{}, r.switchTab(-1)
		case MatchKey(k, Keys.Unwind):
			// Unwind a deep stack back to the root for a quick exit. Only consume it
			// when there's something to unwind, so at the root the key passes through
			// to the active screen instead of being swallowed.
			if len(r.stack) > 1 {
				return ResetToRoot(), true
			}
		}
	}
	return Action{}, false
}

// switchTab moves the active tab by delta (wrapping), but only at the root — when
// drilled into a sub-screen the live stack belongs to the active tab and must not
// be swapped out from under it. The cached root preserves the tab's prior state.
// Reports whether it switched; when it didn't, the key passes through to the
// active screen (so [ / ] can be typed into a form at depth).
func (r *Router) switchTab(delta int) bool {
	if len(r.tabs) < 2 || len(r.stack) != 1 {
		return false
	}
	r.active = (r.active + delta + len(r.tabs)) % len(r.tabs)
	r.stack = []Screen{r.roots[r.active]}
	return true
}

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

// clearOutput empties the output pane and the status line and returns focus to the
// body (the Clear key). No-op without chrome.
func (r *Router) clearOutput() {
	ch := r.sh.Chrome
	if ch == nil {
		return
	}
	if ch.Output != nil {
		ch.Output.Clear()
	}
	if ch.Status != nil {
		ch.Status.Clear()
	}
	ch.outputFocused = false
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
	if _, ok := r.Top().(Overlayer); ok && len(r.stack) >= 2 {
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
	if _, ok := r.Top().(Overlayer); ok && len(r.stack) >= 2 {
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

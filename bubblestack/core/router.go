package core

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	// filter so they don't steal filter keystrokes). globalKey returns a control
	// message (resolved inline) and/or an async cmd (e.g. tea.Quit).
	if key, ok := msg.(tea.KeyMsg); ok {
		if navMsg, cmd, handled := r.globalKey(key); handled {
			r.resolveCtrl(navMsg, &cmds)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
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

	// Control messages that arrive via the queue (an async cmd's result message, a
	// screen's Init, a batch) are applied to the stack synchronously — the same path
	// as a control message a screen returns from Update.
	if _, ok := msg.(ctrlMsg); ok {
		r.resolveCtrl(msg, &cmds)
		r.resize()
		return r, tea.Batch(cmds...)
	}

	// Otherwise it's a screen message: dispatch to the active screen, apply any control
	// message it returns inline (same tick), and hand its async cmd to bubbletea.
	s, navMsg, cmd := r.Top().Update(r.sh, msg)
	r.stack[len(r.stack)-1] = s
	r.resolveCtrl(navMsg, &cmds)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	// Re-lay-out after every message: cheap, and avoids chasing every spot that
	// changes content height (help expansion, log growth, screen switches).
	r.resize()
	return r, tea.Batch(cmds...)
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
		follows, cmd := r.applyCtrl(m)
		if cmd != nil {
			*cmds = append(*cmds, cmd)
		}
		queue = append(queue, follows...)
	}
}

// applyCtrl mutates the stack for one control message, returning any follow-up control
// messages (resolved next by resolveCtrl) and an async cmd (a pushed/replaced screen's
// Init). A non-control message is ignored.
func (r *Router) applyCtrl(m tea.Msg) (follows []tea.Msg, cmd tea.Cmd) {
	switch m := m.(type) {
	case pushMsg:
		r.stack = append(r.stack, m.s)
		cmd = m.s.Init(r.sh)

	case replaceMsg:
		r.stack[len(r.stack)-1] = m.s
		cmd = m.s.Init(r.sh)

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
		// Hand the grouped control messages back to resolveCtrl's worklist, applied
		// in order this same tick.
		follows = m.msgs

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
		// Broadcast the opaque payload to every tab root plus the active tab's deeper
		// screens; each Receiver claims what it recognizes. The router never interprets
		// the payload (no per-notification case). A Receiver may return a control
		// message (e.g. ShowTab to grab focus), resolved in this same tick.
		notify := func(s Screen) {
			if rc, ok := s.(Receiver); ok {
				if f := rc.Receive(r.sh, m.payload); f != nil {
					follows = append(follows, f)
				}
			}
		}
		for i := range r.roots {
			notify(r.roots[i])
		}
		for _, s := range r.stack[1:] { // the active root is already covered via r.roots[active]
			notify(s)
		}

	case MsgThemeChanged:
		// A theme switch repaints the package-level styles (SetTheme), but the
		// cached tab roots baked their delegate/list styles at construction. Rebuild
		// each root from its constructor so the new palette takes; deeper live
		// screens are transient (rebuilt on reopen) and the router-drawn chrome
		// already repaints from the refreshed style vars.
		for i := range r.roots {
			r.roots[i] = r.tabs[i].New(r.sh)
		}
		r.stack[0] = r.roots[r.active]
	}
	return follows, cmd
}

// globalKey handles the keys available in any screen. It returns (navMsg, cmd, true)
// when it consumed the key — navMsg is a control message resolved inline, cmd an async
// cmd (e.g. tea.Quit or an output-scroll cmd) — or (nil, nil, false) to let the active
// screen handle it. Pointer receiver: [ / ] mutate active/stack, which must persist
// back to Update's router.
func (r *Router) globalKey(msg tea.KeyMsg) (tea.Msg, tea.Cmd, bool) {
	k := msg.String()
	if k == "ctrl+c" {
		return nil, tea.Quit, true
	}

	ch := r.sh.Chrome
	outputOn := ch != nil && ch.Output != nil

	// When the output pane holds focus, navigation keys scroll it; everything
	// else either toggles back or clears.
	if outputOn && ch.outputFocused {
		switch {
		case MatchKey(k, Keys.ToggleOutput), MatchKey(k, Keys.Back):
			ch.outputFocused = false
			return nil, nil, true
		case MatchKey(k, Keys.Output):
			ch.Output.Hide()
			ch.outputFocused = false
			return nil, nil, true
		case MatchKey(k, Keys.Clear):
			r.clearOutput()
			return nil, nil, true
		case MatchKey(k, Keys.Quit):
			return nil, tea.Quit, true
		}
		return nil, ch.Output.Update(msg), true
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
			return nil, nil, true
		case MatchKey(k, Keys.Output):
			if !outputOn {
				break
			}
			ch.Output.Toggle()
			if !ch.Output.Shown() {
				ch.outputFocused = false
			}
			return nil, nil, true
		case MatchKey(k, Keys.Clear):
			if ch == nil {
				break
			}
			r.clearOutput()
			return nil, nil, true
		case MatchKey(k, Keys.NextTab):
			return nil, nil, r.switchTab(1)
		case MatchKey(k, Keys.PrevTab):
			return nil, nil, r.switchTab(-1)
		case MatchKey(k, Keys.Unwind):
			// Unwind a deep stack back to the root for a quick exit. Only consume it
			// when there's something to unwind, so at the root the key passes through
			// to the active screen instead of being swallowed.
			if len(r.stack) > 1 {
				return ResetToRoot(), nil, true
			}
		}
	}
	return nil, nil, false
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

// currentMask is the chrome suppression requested by the active (top) screen, or
// the zero mask (hide nothing) when it doesn't implement ChromeMasker.
func (r Router) currentMask() ChromeMask {
	if m, ok := r.Top().(ChromeMasker); ok {
		return m.ChromeMask()
	}
	return ChromeMask{}
}

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
	ch.Status = ""
	ch.outputFocused = false
}

// helpView is the active screen's help bar, suppressed (empty) when the screen masks
// it. helpHeight measures it the same way so the body sizing stays in sync.
func (r Router) helpView(mask ChromeMask) string {
	if mask.Help {
		return ""
	}
	return r.Top().HelpView(r.sh)
}

func (r Router) helpHeight(mask ChromeMask) int { return vheight(r.helpView(mask)) }

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

// topChrome is the persistent chrome above the body: the header box plus the tab
// strip (if any) below it, each gated by the active screen's mask. Its height is
// measured (not a constant) so adding the strip automatically shrinks the body.
func (r Router) topChrome(mask ChromeMask) string {
	var header string
	if !mask.Header && r.sh.Chrome != nil {
		header = r.sh.Chrome.Header.view(r.sh) // nil-receiver safe
	}
	var strip string
	if !mask.TabStrip {
		strip = r.tabStripView()
	}
	switch {
	case header != "" && strip != "":
		return lipgloss.JoinVertical(lipgloss.Left, header, strip)
	case header != "":
		return header
	default:
		return strip
	}
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
	if !mask.Status && ch.Status != "" {
		parts = append(parts, StatusStyle.Render(ch.Status))
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

// bodyHeight is the rows available to the active screen's body: the space between
// the top chrome and the help bar, minus the status/output chrome below the body.
func (r Router) bodyHeight() int {
	mask := r.currentMask()
	h := r.sh.height - vheight(r.topChrome(mask)) - vheight(r.belowChrome(mask)) - r.helpHeight(mask)
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
	r.Top().SetSize(r.sh, r.sh.width, r.bodyHeight())
}

func (r Router) View() string {
	sh := r.sh
	mask := r.currentMask()
	chrome := r.topChrome(mask)
	body := r.Top().View(sh)
	below := r.belowChrome(mask)
	help := r.helpView(mask)
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

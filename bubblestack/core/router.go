package core

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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

	// refreshAction supplies the Action for the global Refresh key (Keys.Refresh).
	// Consumer-set via SetRefreshAction so core names no domain type; nil ⇒ the key
	// is left to the active screen.
	refreshAction func(*Shared) Action
}

// SetRefreshAction wires the consumer's "refresh everything" action to the global
// Refresh key. globalKey invokes it from any screen/depth except while text is being
// captured. Called by the Run facade after NewRouter.
func (r *Router) SetRefreshAction(f func(*Shared) Action) { r.refreshAction = f }

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

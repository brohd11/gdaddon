package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tabEntry is one top-level tab: a title for the strip and its cached root screen.
// The root instance is kept alive across tab switches so each tab preserves its
// list scroll/filter state.
type tabEntry struct {
	title string
	root  screen
}

// router is the top-level tea.Model. It owns the shared chrome state, the set of
// top-level tabs, and the active tab's navigation stack; it renders the tab strip
// + header + help bar (and the output pane) around the active screen, and
// translates navigation messages into stack operations. The active tab's root is
// the permanent bottom of the stack: a pop with a single screen left is ignored.
//
// Tabs are switched with [ / ] only at depth 1, so the live stack always belongs
// to the active tab; there is never a deeper stack to stash when switching.
type router struct {
	sh       *shared
	tabs     []tabEntry
	active   int      // index into tabs of the visible tab
	browseAt int      // tab that owns addon results (msgRootRefresh lands here)
	stack    []screen // live nav stack for the active tab; stack[0] == tabs[active].root
}

func newRouter(sh *shared, tabs []tabEntry) router {
	return router{sh: sh, tabs: tabs, browseAt: 0, stack: []screen{tabs[0].root}}
}

func (r router) top() screen     { return r.stack[len(r.stack)-1] }
func (r router) tabRoot() screen { return r.stack[0] }

func (r router) Init() tea.Cmd { return r.top().Init(r.sh) }

func (r router) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Global keys are handled once, here, for whatever screen is on top: quit,
	// the output-pane focus/scroll mode, and tab/c (gated by the active screen's
	// filter so they don't steal filter keystrokes).
	if key, ok := msg.(tea.KeyMsg); ok {
		if cmd, handled := r.globalKey(key); handled {
			r.resize()
			return r, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.sh.width, r.sh.height = msg.Width, msg.Height
		r.resize()
		return r, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		r.sh.spinner, cmd = r.sh.spinner.Update(msg)
		return r, cmd

	case pushMsg:
		r.stack = append(r.stack, msg.s)
		cmd := msg.s.Init(r.sh)
		r.resize()
		return r, cmd

	case popMsg:
		if len(r.stack) > 1 {
			r.stack = r.stack[:len(r.stack)-1]
		}
		r.resize()
		return r, nil

	case replaceMsg:
		r.stack[len(r.stack)-1] = msg.s
		cmd := msg.s.Init(r.sh)
		r.resize()
		return r, cmd

	case resetToRootMsg:
		r.stack = r.stack[:1]
		r.resize()
		return r, nil

	case msgRootRefresh:
		// Addon results belong to the Browse tab even when triggered from Actions
		// (e.g. Install All): switch to it, unwind to its root, and let that root
		// refresh itself (rootHandler). This is the one deliberate cross-tab coupling
		// — everything else is tab-agnostic.
		r.active = r.browseAt
		r.stack = []screen{r.tabs[r.browseAt].root}
		if h, ok := r.tabRoot().(rootHandler); ok {
			h.handleRoot(r.sh, msg)
		}
		r.resize()
		return r, nil

	case archiveFinishedMsg:
		// Unwind to the versions screen and re-list it so the new archived
		// packages appear.
		for len(r.stack) > 1 {
			if _, ok := r.top().(*versionsScreen); ok {
				break
			}
			r.stack = r.stack[:len(r.stack)-1]
		}
		if v, ok := r.top().(*versionsScreen); ok {
			v.relist()
		}
		r.resize()
		return r, nil
	}

	s, cmd := r.top().Update(r.sh, msg)
	r.stack[len(r.stack)-1] = s
	// Re-lay-out after every message: cheap, and avoids chasing every spot that
	// changes content height (help expansion, log growth, screen switches).
	r.resize()
	return r, cmd
}

// globalKey handles the keys available in any screen. It returns (cmd, true) when
// it consumed the key, or (nil, false) to let the active screen handle it. Pointer
// receiver: [ / ] mutate active/stack, which must persist back to Update's router.
func (r *router) globalKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	k := msg.String()
	if k == "ctrl+c" {
		return tea.Quit, true
	}

	// When the output pane holds focus, navigation keys scroll it; everything
	// else either toggles back or clears.
	if r.sh.focus == focusOutput {
		switch k {
		case "tab", "esc":
			r.sh.focus = focusList
			return nil, true
		case "c":
			r.sh.clearLogs()
			return nil, true
		case "q":
			return tea.Quit, true
		}
		var cmd tea.Cmd
		r.sh.output, cmd = r.sh.output.Update(msg)
		return cmd, true
	}

	// tab jumps into the output pane, c clears the log, [ / ] switch top-level tabs
	// (only at the root, so the live stack always belongs to the active tab), and `
	// unwinds a deep stack back to the root for a quick exit — unless the active
	// screen is capturing filter text.
	if f, ok := r.top().(filterer); !ok || !f.filtering() {
		switch k {
		case "tab":
			if ov, ok := r.top().(outputViewer); ok && ov.wantsOutput() && len(r.sh.logs) > 0 {
				r.sh.focus = focusOutput
				r.sh.output.GotoBottom()
			}
			return nil, true
		case "c":
			r.sh.clearLogs()
			return nil, true
		case "]":
			return nil, r.switchTab(1)
		case "[":
			return nil, r.switchTab(-1)
		case "`":
			// Unwind a deep stack back to the root for a quick exit. Only consume it
			// when there's something to unwind, so at the root the key passes through
			// to the active screen instead of being swallowed.
			if len(r.stack) > 1 {
				return resetToRoot(), true
			}
		}
	}
	return nil, false
}

// switchTab moves the active tab by delta (wrapping), but only at the root — when
// drilled into a sub-screen the live stack belongs to the active tab and must not
// be swapped out from under it. The cached root preserves the tab's prior state.
// Reports whether it switched; when it didn't, the key passes through to the
// active screen (so [ / ] can be typed into a form at depth).
func (r *router) switchTab(delta int) bool {
	if len(r.tabs) < 2 || len(r.stack) != 1 {
		return false
	}
	r.active = (r.active + delta + len(r.tabs)) % len(r.tabs)
	r.stack = []screen{r.tabs[r.active].root}
	return true
}

func (r router) helpHeight() int { return lipgloss.Height(r.top().HelpView(r.sh)) }

// tabStripView renders the top-level tab strip (omitted when there's only one
// tab): the tab titles followed by a full-width rule that delimits it from the
// content below.
func (r router) tabStripView() string {
	if len(r.tabs) < 2 {
		return ""
	}
	tabs := make([]string, len(r.tabs))
	for i, t := range r.tabs {
		if i == r.active {
			tabs[i] = activeTabStyle.Render(t.title)
		} else {
			tabs[i] = tabStyle.Render(t.title)
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
// strip (if any) below it. Its height is measured (not a constant) so adding the
// strip automatically shrinks the body.
func (r router) topChrome() string {
	strip := r.tabStripView()
	if strip == "" {
		return r.sh.headerView()
	}
	return lipgloss.JoinVertical(lipgloss.Left, r.sh.headerView(), strip)
}

// bodyHeight is the rows available to the active screen's body: the space between
// the top chrome and the help bar.
func (r router) bodyHeight() int {
	h := r.sh.height - lipgloss.Height(r.topChrome()) - r.helpHeight()
	if h < 1 {
		h = 1
	}
	return h
}

func (r router) resize() {
	if r.sh.width == 0 {
		return
	}
	// The output viewport is shared chrome, so the router owns its sizing and
	// keeps it pinned to the newest line unless the user is scrolling it.
	r.sh.output.Width = r.sh.outputInnerWidth()
	r.sh.output.Height = r.sh.outputContentHeight()
	r.sh.output.SetContent(r.sh.logContent())
	if r.sh.focus != focusOutput {
		r.sh.output.GotoBottom()
	}
	r.top().SetSize(r.sh, r.sh.width, r.bodyHeight())
}

func (r router) View() string {
	sh := r.sh
	chrome := r.topChrome()
	body := r.top().View(sh)
	// Pad the body so the always-visible help bar sits at the very bottom.
	if pad := (sh.height - lipgloss.Height(chrome) - r.helpHeight()) - lipgloss.Height(body); pad > 0 {
		body = lipgloss.JoinVertical(lipgloss.Left, body, blanks(pad))
	}
	return lipgloss.JoinVertical(lipgloss.Left, chrome, body, r.top().HelpView(sh))
}

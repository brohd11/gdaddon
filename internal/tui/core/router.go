package core

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tabEntry is one top-level tab: a title for the strip and its cached root screen.
// The root instance is kept alive across tab switches so each tab preserves its
// list scroll/filter state.
type TabEntry struct {
	Title string
	Root  Screen
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
	active int      // index into tabs of the visible tab
	stack  []Screen // live nav stack for the active tab; stack[0] == tabs[active].root
}

func NewRouter(sh *Shared, tabs []TabEntry) Router {
	return Router{sh: sh, tabs: tabs, stack: []Screen{tabs[0].Root}}
}

func (r Router) Top() Screen { return r.stack[len(r.stack)-1] }

func (r Router) Init() tea.Cmd { return r.Top().Init(r.sh) }

func (r Router) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		r.sh.Spinner, cmd = r.sh.Spinner.Update(msg)
		return r, cmd

	case pushMsg:
		r.stack = append(r.stack, msg.s)
		cmd := msg.s.Init(r.sh)
		r.resize()
		return r, cmd

	case popMsg:
		for i := 0; i < msg.n && len(r.stack) > 1; i++ {
			r.stack = r.stack[:len(r.stack)-1]
		}
		r.resize()
		return r, nil

	case popToMsg:
		// Always leave the current screen, then stop at the first screen that opts
		// into PopStopper (a command hub), or the root.
		for len(r.stack) > 1 {
			r.stack = r.stack[:len(r.stack)-1]
			if s, ok := r.Top().(PopStopper); ok && s.PopStop() {
				break
			}
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

	case MsgRefresh:
		// A refresh can originate from any tab (Install All from Actions, global
		// Remove, an archive removal, …). Find the tab whose root claims this Target,
		// hand it the message to rebuild itself, and — when Switch is set — make that
		// tab active and unwind it to its root. The router stays tab-agnostic: it asks
		// each root rather than mapping Target → index itself.
		for i := range r.tabs {
			h, ok := r.tabs[i].Root.(RootHandler)
			if !ok || !h.HandleRoot(r.sh, msg) {
				continue
			}
			if msg.Switch {
				r.active = i
				r.stack = []Screen{r.tabs[i].Root}
			}
			break
		}
		r.resize()
		return r, nil
	}

	s, cmd := r.Top().Update(r.sh, msg)
	r.stack[len(r.stack)-1] = s
	// Re-lay-out after every message: cheap, and avoids chasing every spot that
	// changes content height (help expansion, log growth, screen switches).
	r.resize()
	return r, cmd
}

// globalKey handles the keys available in any screen. It returns (cmd, true) when
// it consumed the key, or (nil, false) to let the active screen handle it. Pointer
// receiver: [ / ] mutate active/stack, which must persist back to Update's router.
func (r *Router) globalKey(msg tea.KeyMsg) (tea.Cmd, bool) {
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
	if f, ok := r.Top().(Filterer); !ok || !f.Filtering() {
		switch k {
		case "tab":
			if ov, ok := r.Top().(OutputViewer); ok && ov.WantsOutput() && len(r.sh.Logs) > 0 {
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
				return ResetToRoot(), true
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
func (r *Router) switchTab(delta int) bool {
	if len(r.tabs) < 2 || len(r.stack) != 1 {
		return false
	}
	r.active = (r.active + delta + len(r.tabs)) % len(r.tabs)
	r.stack = []Screen{r.tabs[r.active].Root}
	return true
}

func (r Router) helpHeight() int { return lipgloss.Height(r.Top().HelpView(r.sh)) }

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
// strip (if any) below it. Its height is measured (not a constant) so adding the
// strip automatically shrinks the body.
func (r Router) topChrome() string {
	strip := r.tabStripView()
	if strip == "" {
		return r.sh.headerView()
	}
	return lipgloss.JoinVertical(lipgloss.Left, r.sh.headerView(), strip)
}

// bodyHeight is the rows available to the active screen's body: the space between
// the top chrome and the help bar.
func (r Router) bodyHeight() int {
	h := r.sh.height - lipgloss.Height(r.topChrome()) - r.helpHeight()
	if h < 1 {
		h = 1
	}
	return h
}

func (r Router) resize() {
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
	r.Top().SetSize(r.sh, r.sh.width, r.bodyHeight())
}

func (r Router) View() string {
	sh := r.sh
	chrome := r.topChrome()
	body := r.Top().View(sh)
	// Pad the body so the always-visible help bar sits at the very bottom.
	if pad := (sh.height - lipgloss.Height(chrome) - r.helpHeight()) - lipgloss.Height(body); pad > 0 {
		body = lipgloss.JoinVertical(lipgloss.Left, body, Blanks(pad))
	}
	return lipgloss.JoinVertical(lipgloss.Left, chrome, body, r.Top().HelpView(sh))
}

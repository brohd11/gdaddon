package tui

import (
	"gdaddon/internal/addon"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// router is the top-level tea.Model. It owns the shared chrome state and a stack
// of screens, renders the persistent header + help bar (and the output pane)
// around the active screen, and translates navigation messages into stack
// operations. browse is the permanent root: a pop with a single screen left is
// ignored.
type router struct {
	sh    *shared
	stack []screen
}

func newRouter(sh *shared, root screen) router {
	return router{sh: sh, stack: []screen{root}}
}

func (r router) top() screen { return r.stack[len(r.stack)-1] }

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

	case installDoneMsg:
		r.sh.statusMsg = "updated " + msg.name + " → " + msg.version
		r.applyToRoot(msg.statuses, false)
		r.stack = r.stack[:1]
		r.resize()
		return r, nil

	case installAllDoneMsg:
		r.sh.statusMsg = "install complete"
		r.applyToRoot(msg.statuses, false)
		r.stack = r.stack[:1]
		r.resize()
		return r, nil

	case reloadAddonsMsg:
		r.sh.statusMsg = msg.status
		r.applyToRoot(msg.statuses, true)
		r.stack = r.stack[:1]
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

// applyToRoot updates the browse (root) list with refreshed statuses: rebuild for
// a changed row count (import / new plugin), else update rows in place.
func (r router) applyToRoot(statuses []addon.Status, rebuild bool) {
	b, ok := r.stack[0].(*browseScreen)
	if !ok || statuses == nil {
		return
	}
	if rebuild {
		b.setItems(statuses)
	} else {
		b.applyStatuses(statuses)
	}
}

// globalKey handles the keys available in any screen. It returns (cmd, true) when
// it consumed the key, or (nil, false) to let the active screen handle it.
func (r router) globalKey(msg tea.KeyMsg) (tea.Cmd, bool) {
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

	// tab jumps into the output pane, c clears the log — unless the active screen
	// is capturing filter text.
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
		}
	}
	return nil, false
}

func (r router) helpHeight() int { return lipgloss.Height(r.top().HelpView(r.sh)) }

// bodyHeight is the rows available to the active screen's body: the space between
// the header and the help bar.
func (r router) bodyHeight() int {
	h := r.sh.height - headerHeight - r.helpHeight()
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
	body := r.top().View(sh)
	// Pad the body so the always-visible help bar sits at the very bottom.
	if pad := (sh.height - headerHeight - r.helpHeight()) - lipgloss.Height(body); pad > 0 {
		body = lipgloss.JoinVertical(lipgloss.Left, body, blanks(pad))
	}
	return lipgloss.JoinVertical(lipgloss.Left, sh.headerView(), body, r.top().HelpView(sh))
}

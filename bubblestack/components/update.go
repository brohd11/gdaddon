package components

import (
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// This file collects the shared Update helpers: the small pieces of key-handling
// logic that screens would otherwise re-implement. They dispatch via the central
// core.Keys bindings (see core/keybinds.go) but operate on the reusable list.Model
// / Item pieces, so they live here in components rather than in core (core ←
// components, so core can't name Item or the list helpers).

// Typable is implemented by screens that hold a focused free-text field. When a
// text field has focus, printable keys that alias a navigation binding (e.g. "c"
// for Back, "e" for Select) must be typed, not dispatched. Typing reports whether
// a text field currently holds focus; Input returns it so QueryUpdate can feed the
// keystroke.
type Typable interface {
	Typing() bool
	Input() *textinput.Model
}

// QueryUpdate centralizes the typing-vs-navigation split for any Typable screen.
// When the screen is typing and msg is a character/space keystroke, it feeds the
// input and reports handled=true so the caller skips its keybind switch. Otherwise
// it reports handled=false and the caller runs its normal core.Keys dispatch. Call
// it at the top of Update before the keybind switch. Reused by the Search query
// screen and the New Plugin form so the rule lives in one place. Backspace is
// diverted to the input too (it aliases Back/Keys.Back, so without this it would
// pop the screen instead of deleting a character while typing). The other control
// keys (esc/enter/tab/arrows) are never diverted, so field navigation and cancel
// reach the caller unchanged.
func QueryUpdate(s Typable, msg tea.Msg) (tea.Cmd, bool) {
	if !s.Typing() {
		return nil, false
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch km.Type {
	case tea.KeyRunes, tea.KeySpace, tea.KeyBackspace:
		in := s.Input()
		var cmd tea.Cmd
		*in, cmd = in.Update(msg)
		return cmd, true
	}
	return nil, false
}

// RootUpdate is the shared tab-root key handling, factored out of every tab root's
// Update (project/global/archive/actions/search) since each was identical. While
// the list is filtering, keys go to the list; otherwise Select runs the highlighted
// Item's Pick closure (clearing the status line first); quit is the router's global
// q handler, not handled here. Any other
// key or message falls through to the list. A tab root's Update is then just
// `return s, components.RootUpdate(sh, &s.list, msg)`; roots that also react to
// broadcast notifications keep doing so via core.Receiver.Receive, which the router
// routes separately from Update. Returns the screen's Action.
func RootUpdate(sh *core.Shared, l *list.Model, msg tea.Msg) core.Action {
	if l.FilterState() == list.Filtering {
		var cmd tea.Cmd
		*l, cmd = l.Update(msg)
		return core.Async(cmd)
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		k := key.String()
		switch {
		case core.MatchKey(k, core.Keys.Select):
			if it, ok := l.SelectedItem().(Item); ok && it.Pick != nil {
				sh.ClearStatus()
				return it.Pick(sh)
			}
			return core.Action{}
		default:
			// Let a self-dispatching Item handle its own row keys (e.g. an addon
			// row's "t" → open terminal); unhandled keys fall through to the list.
			if it, ok := l.SelectedItem().(Item); ok && it.Keys != nil {
				if act, handled := it.Keys(sh, k); handled {
					return act
				}
			}
			if WrapNav(l, k) {
				return core.Action{}
			}
		}
	}
	var cmd tea.Cmd
	*l, cmd = l.Update(msg)
	return core.Async(cmd)
}

// WrapNav wraps the cursor at a list boundary: up on the first row selects the
// last, down on the last selects the first. Returns handled=true when it wrapped
// (caller skips forwarding the key to the list). Call only when not filtering, so
// len(l.Items()) is the visible count. Uses the central core.Keys bindings, so the
// wrap follows any added scheme (e.g. wasd); l.Select adjusts pagination itself, so
// wrapping works across pages.
func WrapNav(l *list.Model, k string) bool {
	n := len(l.Items())
	if n < 2 {
		return false
	}
	switch {
	case core.MatchKey(k, core.Keys.Up) && l.Index() == 0:
		l.Select(n - 1)
		return true
	case core.MatchKey(k, core.Keys.Down) && l.Index() == n-1:
		l.Select(0)
		return true
	}
	return false
}

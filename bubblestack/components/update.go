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
// screen and the New Plugin form so the rule lives in one place. Control keys
// (esc/enter/tab/arrows/backspace) are never diverted, so field navigation and
// cursor editing reach the caller / the input's own fall-through unchanged.
func QueryUpdate(s Typable, msg tea.Msg) (tea.Cmd, bool) {
	if !s.Typing() {
		return nil, false
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch km.Type {
	case tea.KeyRunes, tea.KeySpace:
		in := s.Input()
		var cmd tea.Cmd
		*in, cmd = in.Update(msg)
		return cmd, true
	}
	return nil, false
}

// RootUpdate is the shared tab-root key handling, factored out of every tab root's
// Update (project/global/archive/actions/search) since each was identical. While
// the list is filtering, keys go to the list; otherwise Quit quits and Select runs
// the highlighted Item's Pick closure (clearing the status line first). Any other
// key or message falls through to the list. A tab root's Update is then just
// `m, c := components.RootUpdate(sh, &s.list, msg); return s, m, c`; roots that also
// react to broadcast notifications keep doing so via core.Receiver.Receive, which the
// router routes separately from Update. Returns the (sync control msg, async cmd) pair.
func RootUpdate(sh *core.Shared, l *list.Model, msg tea.Msg) (tea.Msg, tea.Cmd) {
	if l.FilterState() == list.Filtering {
		var cmd tea.Cmd
		*l, cmd = l.Update(msg)
		return nil, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		k := key.String()
		switch {
		case core.MatchKey(k, core.Keys.Quit):
			return nil, tea.Quit
		case core.MatchKey(k, core.Keys.Select):
			if it, ok := l.SelectedItem().(Item); ok && it.Pick != nil {
				sh.SetStatus("")
				return it.Pick(sh)
			}
			return nil, nil
		}
	}
	var cmd tea.Cmd
	*l, cmd = l.Update(msg)
	return nil, cmd
}

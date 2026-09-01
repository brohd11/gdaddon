package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// keyMsg builds a tea.KeyPressMsg whose String() is the given keystroke, so tests can
// drive screens from the key strings the dispatch sites match on. It is KeyPressMsg and
// not the tea.KeyMsg interface for the same reason every dispatch site is: that interface
// also covers key releases. v2 ships no string-to-Key parser and the message is a
// Code/Text/Mod struct rather than a named constant per key, so this is the one place a
// test's "ctrl+x" becomes the shape Update sees.
func keyMsg(s string) tea.KeyPressMsg {
	var mod tea.KeyMod
	for stripped := true; stripped; {
		stripped = false
		for _, p := range []struct {
			prefix string
			mod    tea.KeyMod
		}{{"ctrl+", tea.ModCtrl}, {"alt+", tea.ModAlt}, {"shift+", tea.ModShift}} {
			if rest, ok := strings.CutPrefix(s, p.prefix); ok {
				mod, s, stripped = mod|p.mod, rest, true
			}
		}
	}
	if code, ok := namedKeyCodes[s]; ok {
		k := tea.KeyPressMsg{Code: code, Mod: mod}
		// Space is the one named key that also types a character, and the only Text a
		// real terminal reports for it is the blank itself — which Key.String() then
		// special-cases back to "space".
		if code == tea.KeySpace && mod&^tea.ModShift == 0 {
			k.Text = " "
		}
		return k
	}
	if s == "" {
		// The empty keystroke: a key that types nothing, which is what an empty rune
		// slice was in v1 and what callers use to drive "type an empty line".
		return tea.KeyPressMsg{Mod: mod}
	}
	k := tea.KeyPressMsg{Code: []rune(s)[0], Mod: mod}
	// Text is populated only for keys standing for printable characters; a ctrl/alt
	// combo produces none. That is the distinction the input widgets read.
	if mod&^tea.ModShift == 0 {
		k.Text = s
	}
	return k
}

var namedKeyCodes = map[string]rune{
	"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab,
	"backspace": tea.KeyBackspace, "delete": tea.KeyDelete, "space": tea.KeySpace,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
}

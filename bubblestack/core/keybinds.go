package core

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
)

// KeyMap is the single source of truth for every keybinding in the TUI. Each
// field is a key.Binding, which is already "an array of keycodes plus a help
// label" in one value. Dispatch sites match against these with MatchKey, and the
// help bars are built from these with Hint/FullHint — so a binding only ever has
// to be edited here. To add an alternate scheme (e.g. wasd) append the keys to
// the relevant WithKeys list below and it propagates to dispatch, list scrolling,
// the help bar, and the full (?) help automatically.
//
// Convention: ALL key handling — tab/screen Update loops, the shared components,
// and the help bars — dispatches via these bindings with MatchKey and builds help
// with Hint/FullHint; no site matches a raw keycode or key.Type. The shared Update
// helpers that apply these bindings (components.RootUpdate for tab roots,
// components.QueryUpdate for text-entry screens) live in components/update.go,
// since they operate on the reusable list.Model / Item pieces.
type KeyMap struct {
	// navigation
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding

	// actions
	Select key.Binding
	Back   key.Binding
	Quit   key.Binding

	// confirm — Yes carries enter, No carries esc, so a confirm screen matches
	// them directly without consulting Select/Back.
	Yes key.Binding
	No  key.Binding

	// global chrome
	NextTab      key.Binding
	PrevTab      key.Binding
	ToggleOutput key.Binding // focus/unfocus the output pane for scrolling
	Output       key.Binding // show/hide the output box
	Wrap         key.Binding // toggle the output pane's wrap render mode (optional Wrapper)
	Clear        key.Binding
	Unwind       key.Binding
	Refresh      key.Binding // reload all views; action is consumer-supplied

	// form
	NextField key.Binding
	PrevField key.Binding

	// pagination
	PageNext key.Binding
	PagePrev key.Binding
}

// Keys is the active keymap. Edit a WithKeys list here (e.g. add "w"/"s" to
// Up/Down) to rebind everywhere at once. ctrl+c stays a hard-coded hard-quit in
// the router and is intentionally not represented here.
var Keys = KeyMap{
	Up:    key.NewBinding(key.WithKeys("up", "k")),
	Down:  key.NewBinding(key.WithKeys("down", "j")),
	Left:  key.NewBinding(key.WithKeys("left", "h")),
	Right: key.NewBinding(key.WithKeys("right", "l")),

	Select: key.NewBinding(key.WithKeys("enter", "e")),
	Back:   key.NewBinding(key.WithKeys("esc", "backspace", "c")),
	Quit:   key.NewBinding(key.WithKeys("q")),

	Yes: key.NewBinding(key.WithKeys("enter", "y", "Y", "e")),
	No:  key.NewBinding(key.WithKeys("esc", "n", "N", "c")),

	NextTab:      key.NewBinding(key.WithKeys("]", "x", "shift+right", "D")),
	PrevTab:      key.NewBinding(key.WithKeys("[", "z", "shift+left", "A")),
	ToggleOutput: key.NewBinding(key.WithKeys("tab")),
	Output:       key.NewBinding(key.WithKeys("o")),
	Wrap:         key.NewBinding(key.WithKeys("w")),
	Clear:        key.NewBinding(key.WithKeys("C")),
	Unwind:       key.NewBinding(key.WithKeys("`", "r")),
	Refresh:      key.NewBinding(key.WithKeys("u")),

	NextField: key.NewBinding(key.WithKeys("down", "tab")),
	PrevField: key.NewBinding(key.WithKeys("up", "shift+tab")),

	PageNext: key.NewBinding(key.WithKeys("'", "3")),
	PagePrev: key.NewBinding(key.WithKeys(";", "2")),
}

// MatchKey reports whether the pressed key string k is one of binding b's keys.
// It is the string-based analog of key.Matches (which needs a tea.KeyMsg): it
// reads like "k in Keys.Up" and works both in tea.KeyMsg switches (pass
// msg.String()) and in the OnKey closures that already receive a string.
func MatchKey(k string, b key.Binding) bool {
	return slices.Contains(b.Keys(), k)
}

// prettyKey maps raw keycodes to display glyphs so the default bars keep their
// arrow look; unknown keys pass through unchanged.
func prettyKey(k string) string {
	switch k {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "shift+tab":
		return "⇧tab"
	default:
		return k
	}
}

// Hint builds a single help entry from one or more central bindings: the label
// shows only the FIRST keycode of each binding (the always-visible help bar
// rule), while the entry still carries every keycode so it matches and so a
// FullHint over the same binds can expand to all of them. desc is the context
// label (e.g. "option", "remove").
func Hint(desc string, binds ...key.Binding) key.Binding {
	var labels, keys []string
	for _, b := range binds {
		if bk := b.Keys(); len(bk) > 0 {
			labels = append(labels, prettyKey(bk[0]))
			keys = append(keys, bk...)
		}
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(labels, "/"), desc))
}

// FullHint is Hint but the label lists ALL keycodes (the "more help" / full-help
// menu rule): adding "w" to Keys.Up makes this read "↑/k/w" automatically.
func FullHint(desc string, binds ...key.Binding) key.Binding {
	var labels, keys []string
	for _, b := range binds {
		for _, k := range b.Keys() {
			labels = append(labels, prettyKey(k))
			keys = append(keys, k)
		}
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(labels, "/"), desc))
}

// tabHint is the combined "[ ]" tab-switch hint shown by ShortHelp (the two tab
// binds rendered as one entry, keeping the original look).
func tabHint() key.Binding {
	keys := append(append([]string{}, Keys.PrevTab.Keys()...), Keys.NextTab.Keys()...)
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp("[ ]", "tabs"))
}

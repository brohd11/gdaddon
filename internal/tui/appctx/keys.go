package appctx

import "github.com/charmbracelet/bubbles/key"

// appKeyMap collects gdaddon-specific key bindings that aren't part of
// bubblestack's framework keymap (core.Keys in bubblestack/core/keybinds.go).
// Keeping the custom keys in one typed struct — mirroring core.Keys — means a
// rebind is a single edit here rather than hunting string literals across tabs.
type appKeyMap struct {
	Sort     key.Binding // cycle a data list's sort order (Project/Global/Archive)
	Terminal key.Binding // open a terminal at an installed addon's install path (Project)
}

// AppKeys is the active custom keymap. Edit a WithKeys list here to rebind; the
// tabs match against these bindings (via core.MatchKey), so nothing else changes.
var AppKeys = appKeyMap{
	Sort:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "sort")),
	Terminal: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "terminal")),
}

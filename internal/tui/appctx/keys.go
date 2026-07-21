package appctx

import "github.com/charmbracelet/bubbles/key"

// appKeyMap collects gdaddon-specific key bindings that aren't part of
// bubblestack's framework keymap (core.Keys in bubblestack/core/keybinds.go).
// Keeping the custom keys in one typed struct — mirroring core.Keys — means a
// rebind is a single edit here rather than hunting string literals across tabs.
type appKeyMap struct {
	Sort     key.Binding // cycle a data list's sort order (Project/Global/Archive)
	Terminal key.Binding // open a terminal at an installed addon's install path (Project)
	Fetch    key.Binding // git-fetch every project git checkout, refreshing its ahead/behind (Project)
	Git      key.Binding // open the highlighted addon's Git page (Project)
	Diff     key.Binding // open the highlighted addon's diff list (Project; git checkouts only)
	GitAll   key.Binding // open the project-wide (all-repos) Git page (Project)
	RootGit  key.Binding // open the project repo's own Git page (Project)
}

// AppKeys is the active custom keymap. Edit a WithKeys list here to rebind; the
// tabs match against these bindings (via core.MatchKey), so nothing else changes.
var AppKeys = appKeyMap{
	Sort:     key.NewBinding(key.WithKeys("i"), key.WithHelp("s", "sort")),
	Terminal: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "terminal")),
	Fetch:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fetch")),
	// v/V rather than g/G: bubbles binds g/G to jump-to-top/bottom on every list, and we keep
	// that consistent across tabs rather than making one list behave differently. v = version
	// control. ctrl+v is the project repo itself — V puts it in the batch, ctrl+v works it alone.
	Git:     key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "git")),
	Diff:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
	GitAll:  key.NewBinding(key.WithKeys("V"), key.WithHelp("V", "git all")),
	RootGit: key.NewBinding(key.WithKeys("ctrl+v"), key.WithHelp("ctrl+v", "root git")),
}

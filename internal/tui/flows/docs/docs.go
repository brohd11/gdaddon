// Package docs is gdaddon's in-TUI manual flow: the Docs index and the first-run
// welcome popup that offers it. The manual's pages themselves live in the repo's
// doc folder (gdaddon/doc/embedded, exposed by the doc package) so they're easy to
// find and edit; the parse/render/index machinery is shared bubblestack machinery
// (components). This package owns only the TUI glue and the welcome copy.
//
// It's a flow rather than a tab because two layers reach it: the Actions tab (the
// menu row) and tui.Run (the first-run popup, via WelcomeCmd).
package docs

import (
	"github.com/brohd11/gdaddon/doc"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "charm.land/bubbletea/v2"
)

// Pages returns the embedded manual pages in filename order (see gdaddon/doc).
func Pages() []components.DocPage { return doc.Pages() }

// Index is the docs menu: one self-dispatching row per page, each pushing its own reader.
func Index() *components.PickerScreen { return components.DocsIndex("Docs", "Docs", Pages()) }

// Welcome is the first-run popup: a modal over whatever tab the TUI opened on, offering
// the docs. Enter opens the index in its place (Replace, so esc from the index lands
// back on the tab rather than re-showing the popup); esc dismisses it.
func Welcome() *components.DialogScreen {
	return components.CreatePopup(
		"Welcome to gdaddon",
		"gdaddon installs and tracks Godot addons from a manifest.\n\n"+
			"Set up ~/.gdaddon for its config and archive.\n\n"+
			"Docs are available any time under Actions ▸ Docs.",
		core.Replace(Index()),
		core.Hint("open docs", core.Keys.Yes),
		core.Hint("dismiss", core.Keys.No),
	)
}

// WelcomeCmd shows the welcome popup once the router is up. It's a cmd (not an Action)
// so it can ride bubblestack.Config.Init alongside the startup update check; the router
// applies the Action it returns as a message.
func WelcomeCmd() tea.Cmd {
	return func() tea.Msg { return core.Push(Welcome()) }
}

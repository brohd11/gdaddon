// Package docs is gdaddon's manual flow: the Docs index and the first-run welcome popup.
// The pages live in gdaddon/doc; rendering is bubblestack's. A flow because both the
// Actions tab and tui.Run reach it.
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

// Welcome is the first-run popup offering the docs. Enter replaces it with the index (so
// esc returns to the tab); esc dismisses it.
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

// WelcomeCmd shows the welcome popup once the router is up, as a cmd for Config.Init.
func WelcomeCmd() tea.Cmd {
	return func() tea.Msg { return core.Push(Welcome()) }
}

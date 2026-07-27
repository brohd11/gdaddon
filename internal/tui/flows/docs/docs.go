// Package docs is gdaddon's in-TUI manual: a set of markdown pages compiled into the
// binary, browsed from Actions ▸ Docs, and offered to first-run users by a welcome popup.
//
// It's a flow rather than a tab because two layers reach it: the Actions tab (the menu
// row) and tui.Run (the first-run popup, via WelcomeCmd).
//
// The parse/render/index machinery is shared — it lives in bubblestack/components (used by
// repoview too); this package owns only gdaddon's embedded pages and the welcome copy.
// Adding a page is dropping a numbered .md file into pages/ — no code change. The
// filename orders it, the first "# " heading is its title, and the first line after that
// heading is its one-line description in the index.
package docs

import (
	"embed"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

//go:embed pages/*.md
var pagesFS embed.FS

// Pages returns the embedded pages in filename order.
func Pages() []components.DocPage { return components.ParseDocPages(pagesFS, "pages") }

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

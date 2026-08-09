// Package doc ships gdaddon's user manual: the markdown pages in embedded/,
// compiled into the binary and browsed in-app via Actions ▸ Docs (the TUI flow
// lives in internal/tui/flows/docs). The pages sit here — the repo's doc folder —
// rather than inside the TUI package so they're easy to find and edit; go:embed
// can't reach a parent directory, so the embed must live at this level.
//
// Adding a page is dropping a numbered .md into embedded/ — no code change. The
// filename orders it, the first "# " heading is its title, and the first line
// under that heading is its one-line description in the index.
package doc

import (
	"embed"

	"github.com/brohd11/bubblestack/components"
)

//go:embed embedded/*.md
var pagesFS embed.FS

// Pages returns the embedded manual pages in filename order.
func Pages() []components.DocPage { return components.ParseDocPages(pagesFS, "embedded") }

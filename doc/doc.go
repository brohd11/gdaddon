// Package doc embeds gdaddon's manual (embedded/*.md), browsed via Actions ▸ Docs. It sits
// here because go:embed cannot reach a parent directory. Add a page by dropping a numbered
// .md into embedded/: the filename orders it, the first "# " heading is its title, and the
// line under it its description.
package doc

import (
	"embed"

	"github.com/brohd11/bubblestack/components"
)

//go:embed embedded/*.md
var pagesFS embed.FS

// Pages returns the embedded manual pages in filename order.
func Pages() []components.DocPage { return components.ParseDocPages(pagesFS, "embedded") }

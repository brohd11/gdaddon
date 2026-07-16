package app

import (
	"fmt"

	"github.com/brohd11/bubblestack/core"
)

// Header renders repoview's persistent context box: the scanned root and how many git
// checkouts it holds. Wired onto core.Chrome.Header, so the router draws it above every screen.
func Header(sh *core.Shared) string {
	c := Of(sh)
	inner := core.HeaderInnerWidth(sh.Width())
	valWidth := inner - 8 // minus the "Repos: " label
	body := core.Label("Root:  ") + core.Value(core.TruncLeft(c.Root, valWidth)) + "\n" +
		core.Label("Repos: ") + core.Value(fmt.Sprintf("%d git checkout(s) · depth ≤ %d", len(c.Repos), c.Depth))
	return core.HeaderBox(sh.Width(), body)
}

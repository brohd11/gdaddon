package app

import (
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"
)

// Ctx is repoview's app context, stored on core.Shared.App and recovered with Of. It holds the
// scan root/depth and the repos found by the last scan. There is no manifest — the list is
// simply whatever a fresh scan of Root turns up, which is the whole point of the tool.
type Ctx struct {
	Root  string
	Depth int
	Repos []repo.Repo
}

// New builds the context and performs the initial scan, so the first screen has rows to show.
func New(root string, depth int) *Ctx {
	c := &Ctx{Root: root, Depth: depth}
	c.Scan()
	return c
}

// Of recovers the repoview context from a Shared. Screens call c := app.Of(sh).
func Of(sh *core.Shared) *Ctx { return core.App[Ctx](sh) }

// Scan re-reads every git checkout under Root — branch, upstream divergence, and dirty state
// per repo (all local reads). A scan error leaves the previous list intact rather than blanking
// the screen.
func (c *Ctx) Scan() {
	if repos, err := repo.Scan(c.Root, c.Depth); err == nil {
		c.Repos = repos
	}
}

// RescanMsg is repoview's "reload yourself" broadcast: the repo list re-scans from disk on it.
// The Refresh action and the global Refresh key ('r') raise it; the shared git flows raise
// repoui.RefreshMsg after an op, which the screen treats the same way.
type RescanMsg struct{}

// Receive handles app-level broadcasts. On a theme change it rebuilds the cached root so it
// re-bakes its list/delegate styles from the new palette (the router-drawn chrome repaints on
// its own). Everything else is handled by the screen.
func (c *Ctx) Receive(sh *core.Shared, payload any) core.Action {
	switch payload.(type) {
	case core.MsgThemeChanged:
		return core.RefreshRoots()
	}
	return core.Action{}
}

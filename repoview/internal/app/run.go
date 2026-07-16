package app

import (
	"github.com/brohd11/bubblestack"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// Run scans root for git repos and launches the repoview TUI: a single repo-list tab (so
// bubblestack draws no tab strip), the shared git screens reached from it, the persistent
// header, a log/output pane (the git flows stream into it), and a status line. The persisted
// theme, if any, is applied at startup; the global Refresh key rescans.
func Run(root string, depth int) error {
	return bubblestack.Run(bubblestack.Config{
		App:    New(root, depth),
		Header: Header,
		Output: components.NewLogPane(),
		Status: components.NewStatusLine(),
		Theme:  loadTheme(),
		Tabs: []bubblestack.TabEntry{
			{Title: "Repos", New: func(sh *core.Shared) core.Screen { return NewReposScreen(sh) }},
		},
		RefreshAction: func(sh *core.Shared) core.Action { return refreshAction(sh) },
	})
}

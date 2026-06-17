// The package overview and architecture live in doc.go.
package tui

import (
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/tabs/actions"
	"gdaddon/internal/tui/tabs/archive"
	"gdaddon/internal/tui/tabs/global"
	"gdaddon/internal/tui/tabs/project"
	"gdaddon/internal/tui/tabs/search"

	"github.com/brohd11/bubblestack"
	"github.com/brohd11/bubblestack/components"
)

// Run wires the tabs and blocks until the user quits. Tab roots are built lazily by
// the router (after the theme is applied), so each tab reads its own state when
// constructed; nothing is inspected here.
func Run(manifestPath, projectRoot string) error {
	return bubblestack.Run(bubblestack.Config{
		App:    appctx.New(manifestPath, projectRoot),
		Header: appctx.Header,
		Output: components.NewLogPane(),
		Theme:  "mono",
		Tabs: []bubblestack.TabEntry{
			{Title: appctx.TitleProject, New: func(sh *bubblestack.Shared) bubblestack.Screen { return project.NewProjectScreen(sh) }},
			{Title: appctx.TitleGlobal, New: func(sh *bubblestack.Shared) bubblestack.Screen { return global.NewGlobalScreen() }},
			{Title: appctx.TitleArchive, New: func(sh *bubblestack.Shared) bubblestack.Screen { return archive.NewArchiveScreen() }},
			{Title: appctx.TitleActions, New: func(sh *bubblestack.Shared) bubblestack.Screen { return actions.NewActionsScreen() }},
			{Title: appctx.TitleSearch, New: func(sh *bubblestack.Shared) bubblestack.Screen { return search.NewSearchScreen() }},
		},
	})
}

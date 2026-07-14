// The package overview and architecture live in doc.go.
package tui

import (
	"gdaddon/internal/config"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/docs"
	"gdaddon/internal/tui/tabs/actions"
	"gdaddon/internal/tui/tabs/archive"
	"gdaddon/internal/tui/tabs/global"
	"gdaddon/internal/tui/tabs/project"
	"gdaddon/internal/tui/tabs/search"
	"gdaddon/internal/tui/tabs/sets"

	"github.com/brohd11/bubblestack"
	"github.com/brohd11/bubblestack/components"

	tea "github.com/charmbracelet/bubbletea"
)

// Run wires the tabs and blocks until the user quits. Tab roots are built lazily by
// the router (after the theme is applied), so each tab reads its own state when
// constructed; nothing is inspected here.
//
// firstRun (gdaddon had to create ~/.gdaddon) adds the docs welcome popup to the
// startup hook — the one moment we know the user has never seen the tool.
func Run(projectRoot, version string, firstRun bool) error {
	theme := "mono"
	if cfg, err := config.Load(); err == nil && cfg.CurrentTheme != "" {
		theme = cfg.CurrentTheme
	}
	return bubblestack.Run(bubblestack.Config{
		App:    appctx.New(projectRoot, version),
		Header: appctx.Header,
		Output: components.NewLogPane(),
		Status: components.NewStatusLine(),
		Theme:  theme,
		Init: func(sh *bubblestack.Shared) tea.Cmd {
			cmds := []tea.Cmd{appctx.SelfUpdateCheckCmd(sh)}
			if firstRun {
				cmds = append(cmds, docs.WelcomeCmd())
			}
			return tea.Batch(cmds...)
		},
		RefreshAction: func(sh *bubblestack.Shared) bubblestack.Action {
			return appctx.RefreshAll()
		},
		Tabs: []bubblestack.TabEntry{
			{Title: appctx.TitleProject, New: func(sh *bubblestack.Shared) bubblestack.Screen { return project.NewProjectScreen(sh) }},
			{Title: appctx.TitleGlobal, New: func(sh *bubblestack.Shared) bubblestack.Screen { return global.NewGlobalScreen(sh) }},
			{Title: appctx.TitleSets, New: func(sh *bubblestack.Shared) bubblestack.Screen { return sets.NewSetsScreen(sh) }},
			{Title: appctx.TitleArchive, New: func(sh *bubblestack.Shared) bubblestack.Screen { return archive.NewArchiveScreen() }},
			{Title: appctx.TitleActions, New: func(sh *bubblestack.Shared) bubblestack.Screen { return actions.NewActionsScreen(sh) }},
			{Title: appctx.TitleSearch, New: func(sh *bubblestack.Shared) bubblestack.Screen { return search.NewSearchScreen() }},
		},
	})
}

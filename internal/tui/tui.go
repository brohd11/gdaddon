// The package overview and architecture live in doc.go.
package tui

import (
	"fmt"

	arch "github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/docs"
	"github.com/brohd11/gdaddon/internal/tui/sysopen"
	"github.com/brohd11/gdaddon/internal/tui/tabs/actions"
	"github.com/brohd11/gdaddon/internal/tui/tabs/archive"
	"github.com/brohd11/gdaddon/internal/tui/tabs/global"
	"github.com/brohd11/gdaddon/internal/tui/tabs/project"
	"github.com/brohd11/gdaddon/internal/tui/tabs/search"
	"github.com/brohd11/gdaddon/internal/tui/tabs/sets"

	"github.com/brohd11/bubblestack"
	"github.com/brohd11/bubblestack/components"

	tea "charm.land/bubbletea/v2"
)

// Run wires the tabs and blocks until the user quits. Tab roots are built lazily by
// the router (after the theme is applied), so each tab reads its own state when
// constructed; nothing is inspected here.
//
// firstRun (gdaddon had to create ~/.gdaddon) adds the docs welcome popup to the
// startup hook — the one moment we know the user has never seen the tool.
func Run(projectRoot, version string, firstRun bool) error {
	return bubblestack.Run(bubblestack.Config{
		App:    appctx.New(projectRoot, version),
		Header: appctx.Header,
		// A header click opens the project repo's own Git page, same as ctrl+v.
		HeaderClick: func(sh *bubblestack.Shared, _, _ int) bubblestack.Action { return project.RootGitAction(sh) },
		Output:      components.NewLogPane(),
		Status:      components.NewStatusLine(),
		// Theme is left unset so bubblestack.Run loads the shared ~/.bubblestack theme.
		Init: func(sh *bubblestack.Shared) tea.Cmd {
			// Non-fatal domain problems reach the log pane: load failures recorded
			// during appctx.New (which ran before the router existed), and archive
			// index-refresh failures (see arch.Logf).
			for _, e := range appctx.Of(sh).DrainLoadErrs() {
				sh.Log(e)
			}
			arch.Logf = func(format string, args ...any) { sh.Log(fmt.Sprintf(format, args...)) }
			cmds := []tea.Cmd{appctx.SelfUpdateCheckCmd(sh)}
			if firstRun {
				cmds = append(cmds, docs.WelcomeCmd())
			}
			return tea.Batch(cmds...)
		},
		RefreshAction: func(sh *bubblestack.Shared) bubblestack.Action {
			return appctx.RefreshAll()
		},
		TerminalAction:       func(dir string) bubblestack.Action { return sysopen.TerminalInline(dir) },
		TerminalWindowAction: func(dir string) bubblestack.Action { return sysopen.Terminal(dir) },
		OpenDirAction:        func(dir string) bubblestack.Action { return sysopen.Path(dir, false) },
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

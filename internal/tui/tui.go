// The package overview and architecture live in doc.go.
package tui

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/core"
	"gdaddon/internal/tui/tabs/actions"
	"gdaddon/internal/tui/tabs/archive"
	"gdaddon/internal/tui/tabs/global"
	"gdaddon/internal/tui/tabs/project"

	tea "github.com/charmbracelet/bubbletea"
)

// Run loads the manifest, builds the program, and blocks until the user quits.
func Run(manifestPath, projectRoot string) error {
	statuses, err := addon.Inspect(manifestPath, projectRoot)
	if err != nil {
		return err
	}

	sh := core.NewShared(manifestPath, projectRoot)
	tabs := []core.TabEntry{
		{Title: "Project", Root: project.NewProjectScreen(statuses)},
		{Title: "Global", Root: global.NewGlobalScreen()},
		{Title: "Archive", Root: archive.NewArchiveScreen()},
		{Title: "Actions", Root: actions.NewActionsScreen()},
	}
	r := core.NewRouter(sh, tabs)
	_, err = tea.NewProgram(r, tea.WithAltScreen()).Run()
	return err
}

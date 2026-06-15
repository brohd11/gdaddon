// The package overview and architecture live in doc.go.
package tui

import (
	"gdaddon/internal/addon"

	tea "github.com/charmbracelet/bubbletea"
)

// Run loads the manifest, builds the program, and blocks until the user quits.
func Run(manifestPath, projectRoot string) error {
	statuses, err := addon.Inspect(manifestPath, projectRoot)
	if err != nil {
		return err
	}

	sh := newShared(manifestPath, projectRoot)
	tabs := []tabEntry{
		{title: "Browse", root: newBrowseScreen(statuses)},
		{title: "Actions", root: newActionsScreen()},
	}
	r := newRouter(sh, tabs)
	_, err = tea.NewProgram(r, tea.WithAltScreen()).Run()
	return err
}

// add targets for the New Plugin target toggle.
const (
	targetProject = iota
	targetGlobal
)

// rows of the New Plugin form (url/name/path text fields + the target toggle).
// URL is first because it's the only mandatory field.
const (
	fldURL = iota
	fldName
	fldPath
	fldTarget
	fldCount
)

// focusArea tracks which pane receives navigation keys.
type focusArea int

const (
	focusList focusArea = iota
	focusOutput
)

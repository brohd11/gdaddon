// Package tui implements the interactive bubbletea front-end for browsing
// addons, picking a remote version, and installing/updating. It renders state
// from the addon package and turns install progress into bubbletea messages.
//
// The UI is a router (router.go) holding a stack of screens (screen.go). The
// router owns the persistent chrome — header, help bar, output/log pane — in
// shared (shared.go); each screen (screen_*.go) owns its own body, keys, and
// help, and navigates by returning the stack commands in nav.go.
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
	r := newRouter(sh, newBrowseScreen(statuses))
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

// headerHeight is the persistent context box above the body.
const headerHeight = 5 // border (2) + 3 content lines

// focusArea tracks which pane receives navigation keys.
type focusArea int

const (
	focusList focusArea = iota
	focusOutput
)

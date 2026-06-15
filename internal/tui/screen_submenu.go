package tui

import (
	"gdaddon/internal/addon"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// newSubmenuScreen builds the leaf picker — a release's assets, or HEAD's
// branches — as a pickerScreen: enter installs the chosen item, 'a' archives it.
// It's a thin wrapper that wires items + behavior into the reusable picker.
func newSubmenuScreen(selected addon.Addon, local string, items []list.Item, title string) *pickerScreen {
	return newPicker(items, pickerOpts{
		title: title,
		help:  []key.Binding{archiveKey},
		onSelect: func(sh *shared, it list.Item) tea.Cmd {
			v, ok := it.(versionItem)
			if !ok {
				return nil
			}
			return push(newInstallConfirm(selected, local, v))
		},
		onKey: archiveKeyHandler(selected, local),
	})
}

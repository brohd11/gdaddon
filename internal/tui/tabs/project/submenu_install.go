package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// newInstallPicker builds the leaf install picker — a release's assets, or HEAD's
// branches — as a pickerScreen: enter installs the chosen item, 'a' archives it.
// It's a thin wrapper that wires items + behavior into the reusable picker, reached
// from the addon submenu's Install command.
func newInstallPicker(selected addon.Addon, local string, items []list.Item, title string) *components.PickerScreen {
	return components.NewPicker(items, components.PickerOpts{
		Title: title,
		Help:  []key.Binding{archiveKey},
		OnSelect: func(sh *core.Shared, it list.Item) tea.Cmd {
			v, ok := it.(versionItem)
			if !ok {
				return nil
			}
			return core.Push(newInstallConfirm(selected, local, v))
		},
		OnKey: archiveKeyHandler(selected, local),
	})
}

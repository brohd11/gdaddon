package global

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"github.com/brohd/bubblestack/components"
	"github.com/brohd/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// newSubmenuScreen builds the per-plugin command submenu as a reusable picker.
// Each row carries its own Pick, so new global commands are added as rows here.
func newSubmenuScreen(g globalItem) *components.PickerScreen {
	items := []list.Item{
		components.Item{
			Name: "⬇ Import to Project",
			Desc: "add this plugin to the project manifest",
			Pick: func(sh *core.Shared) tea.Cmd { return importToProject(sh, g) },
		},
		components.Item{
			Name: "✗ Remove",
			Desc: "remove from the global list (and optionally its archive)",
			Pick: func(sh *core.Shared) tea.Cmd { return core.Push(newRemoveConfirm(g)) },
		},
	}
	return components.NewPicker(items, components.PickerOpts{Title: g.name})
}

// importToProject copies the global entry into the project manifest, then refreshes
// the Project tab, which reloads the new row from the manifest.
func importToProject(sh *core.Shared, g globalItem) tea.Cmd {
	if err := addon.AddEntry(appctx.Of(sh).ManifestPath, g.name, g.url, g.path); err != nil {
		sh.StatusMsg = "error: " + err.Error()
		return core.ResetToRoot()
	}
	return core.Refresh(appctx.Project, true, "imported "+g.name)
}

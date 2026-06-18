package global

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newSubmenuScreen builds the per-plugin command submenu as a reusable picker.
// Each row carries its own Pick, so new global commands are added as rows here.
func newSubmenuScreen(g globalItem) *components.PickerScreen {
	items := []list.Item{
		components.Item{
			Name: "⬇ Import to Project",
			Desc: "add this plugin to the project manifest",
			Pick: func(sh *core.Shared) core.Action { return importToProject(sh, g) },
		},
		components.Item{
			Name: "✗ Remove",
			Desc: "remove from the global list (and optionally its archive)",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newRemoveConfirm(g)) },
		},
	}
	return components.NewPicker(items, components.PickerOpts{Title: g.name})
}

// importToProject copies the global entry into the project manifest, then broadcasts
// ProjectDirty (Focus false, so the Project list reloads silently without leaving the
// Global tab) and pops the submenu back to the Global list — handy for importing several.
func importToProject(sh *core.Shared, g globalItem) core.Action {
	if err := addon.AddEntry(appctx.Of(sh).ManifestPath, g.name, g.url, g.path); err != nil {
		sh.SetStatus("error: " + err.Error())
		return core.ResetToRoot()
	}
	return core.Seq(
		core.SetStatus("imported "+g.name),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.Pop(),
	)
}

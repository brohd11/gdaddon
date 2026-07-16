package app

import (
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// actionsMenu is the small Actions picker opened with "a": switch the theme, or refresh
// (rescan the directory). PopStop makes it the hub its sub-flows (the theme picker) return to.
// Self-update is deferred until repoview has a release pipeline, so it isn't listed yet.
func actionsMenu(sh *core.Shared) *components.PickerScreen {
	items := []list.Item{
		components.Item{
			Name: "◑ Theme",
			Desc: "switch the color theme",
			Pick: func(sh *core.Shared) core.Action { return core.Push(themePicker()) },
		},
		components.Item{
			Name: "⟳ Refresh",
			Desc: "rescan the directory and refresh git state",
			Pick: func(sh *core.Shared) core.Action { return refreshAction(sh) },
		},
	}
	return components.NewPicker(items, components.PickerOpts{
		Title:   "Actions",
		Crumb:   "Actions",
		PopStop: true,
	})
}

// refreshAction rescans and rebuilds the list — the action both the Actions ▸ Refresh row and
// the global Refresh key ("r") fire. It broadcasts RescanMsg (the screen re-scans on it) and
// sets the status line.
func refreshAction(sh *core.Shared) core.Action {
	return core.Seq(
		core.PropagateAll(RescanMsg{}),
		core.SetStatus("Refreshed"),
	)
}

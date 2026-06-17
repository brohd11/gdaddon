package project

import (
	"gdaddon/internal/addon"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newSubmenuScreen builds the per-addon command submenu (the screen reached by
// pressing enter on an addon row). Install opens the version-fetch flow; Archive
// (offered only when the addon is installed) opens the archive submenu; Remove
// opens the remove confirm. Each row carries its own Pick.
func newSubmenuScreen(st addon.Status) *components.PickerScreen {
	a, local := st.Addon, st.LocalVersion

	items := []list.Item{
		components.Item{
			Name: "↧ Install / update",
			Desc: "pick a version, branch, or asset to install",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newReleasesLoading(a, local)) },
		},
	}
	if st.Present() {
		items = append(items, components.Item{
			Name: "📦 Archive",
			Desc: "save a local copy of this addon",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newArchiveSubmenu(st)) },
		})
	}
	items = append(items, components.Item{
		Name: "✗ Remove",
		Desc: "remove from the project (and optionally delete files)",
		Pick: func(sh *core.Shared) core.Action { return core.Push(newRemoveConfirm(st)) },
	})

	return components.NewPicker(items, components.PickerOpts{
		Title:   core.HeaderTitle(a.Name, local, ""),
		PopStop: true, // the per-addon command hub: sub-flows PopTo() back here
	})
}

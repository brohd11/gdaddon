package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/packages"

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
			Pick: func(sh *core.Shared) core.Action {
				return core.Push(packages.BrowseRepo(a.URL, packages.BrowseOpts{
					Source:      packages.SourceAll,
					IncludeHEAD: true,
					Endpoint:    installEndpoint(a, local),
				}))
			},
		},
	}
	if a.URL != "" && !addon.InGlobalList(a.URL) {
		items = append(items, components.Item{
			Name: "⬆ Export to Global",
			Desc: "add this plugin to your global library (~/.gdaddon)",
			Pick: func(sh *core.Shared) core.Action { return exportToGlobal(sh, a) },
		})
	}

	if st.Present() {
		items = append(items, components.Item{
			Name: "📦 Archive",
			Desc: "save a local copy of this addon",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newArchiveSubmenu(st)) },
		})
		items = append(items, components.Item{
			Name: "✗ Remove",
			Desc: "remove from the project (and optionally delete files)",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newRemoveConfirm(st)) },
		})
	}

	return components.NewPicker(items, components.PickerOpts{
		Title:   core.HeaderTitle(a.Name, local, ""),
		PopStop: true, // the per-addon command hub: sub-flows PopTo() back here
	})
}

// exportToGlobal copies the project addon into the global list, stripping the
// (often release/archive-pinned) url down to its canonical repo url and dropping
// the project-relative path — global entries are url-only. It then broadcasts
// GlobalDirty (Focus false → the Global list reloads silently without leaving the
// Project tab) and pops the submenu back. The row that triggers this is only shown
// when the repo isn't already in the global list (addon.InGlobalList).
func exportToGlobal(sh *core.Shared, a addon.Addon) core.Action {
	url := a.URL
	if stripped, err := source.RepoURL(a.URL); err == nil {
		url = stripped
	}
	globalPath, err := addon.GlobalListPath()
	if err == nil {
		err = addon.AddEntry(globalPath, a.Name, url, "")
	}
	if err != nil {
		return core.Seq(
			core.SetStatusAndLog("error: "+err.Error()),
			core.ResetToRoot(),
		)
	}
	return core.Seq(
		core.SetStatus("added "+a.Name+" to global list"),
		core.PropagateAll(appctx.GlobalDirty{}),
		core.Pop(),
	)
}

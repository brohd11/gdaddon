package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/editmanifest"
	"gdaddon/internal/tui/flows/packages"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newSubmenuScreen builds the per-addon command submenu (the screen reached by
// pressing enter on an addon row). Install opens the version-fetch flow; Archive
// (offered only when the addon is installed) opens the archive submenu; Remove
// opens the remove confirm. Each row carries its own Pick.
func newSubmenuScreen(st addon.Status, sh *core.Shared) *components.PickerScreen {
	a, local := st.Addon, st.LocalVersion
	c := appctx.Of(sh)

	items := []list.Item{
		components.Item{
			Name: "↧ Install / update",
			Desc: "pick a version, branch, or asset to install",
			Pick: func(sh *core.Shared) core.Action {
				// BrowseRepo lists store releases for a store url and git versions
				// otherwise; installEndpoint branches on the same to build the right
				// confirm/task.
				return core.Push(packages.BrowseRepo(a.URL, packages.BrowseOpts{
					Source:         packages.SourceAll,
					IncludeHEAD:    true,
					Endpoint:       installEndpoint(a, local),
					ArchivedMarker: "(archived)",
				}))
			},
		},
	}
	// Offered only when the addon is installed and actually has unsatisfied deps
	// (the cached check), so a fully-satisfied addon doesn't show a no-op action.
	if a.URL != "" && st.Present() && len(c.DepChecks[a.Name]) > 0 {
		items = append(items, components.Item{
			Name: "⛓ Get dependencies",
			Desc: "add this plugin's missing dependencies to the manifest (Install All to install)",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newGetDepsLoading(st, sh)) },
		})
	}
	if a.URL != "" && !st.InGlobal(c.GlobalAddons) {
		items = append(items, components.Item{
			Name: "⬆ Export to Global",
			Desc: "add this plugin to your global library (~/.gdaddon)",
			Pick: func(sh *core.Shared) core.Action { return exportToGlobal(sh, a) },
		})
	}
	items = append(items, components.Item{
		Name: "⛃ Archive",
		Desc: "browse the repo's versions and save a local copy",
		Pick: func(sh *core.Shared) core.Action { return core.Push(newArchiveSubmenu(st, sh)) },
	})
	items = append(items, components.Item{
		Name: "✎ Edit Manifest",
		Desc: "edit this plugin's manifest entry (url, path, version, tag, clone)",
		Pick: func(sh *core.Shared) core.Action {
			return core.Push(editmanifest.New(appctx.Of(sh).ManifestPath, a, appctx.ProjectDirty{}, false))
		},
	})
	items = append(items, components.Item{
		Name: "✗ Remove",
		Desc: "remove from the project (and optionally delete files)",
		Pick: func(sh *core.Shared) core.Action { return core.Push(newRemoveConfirm(st)) },
	})

	return components.NewPicker(items, components.PickerOpts{
		// Crumb:   "Plugin",
		Title:   a.Name,
		PopStop: true, // the per-addon command hub: sub-flows PopTo() back here
	})
}

// exportToGlobal copies the project addon into the global list, stripping the
// (often release/archive-pinned) url down to its canonical repo url and carrying
// the project-relative path along as the global entry's remembered default. It then broadcasts
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
		err = addon.AddEntry(globalPath, a.Name, url, a.Path)
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

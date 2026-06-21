package global

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/editmanifest"
	pck "gdaddon/internal/tui/flows/packages"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// InProject reports whether this global plugin's repo is already present in the
// given project addon list (matched by source.RepoID).
func (g globalItem) InProject(addons []addon.Addon) bool {
	id, err := source.RepoID(g.url)
	if err != nil {
		return false
	}
	for _, a := range addons {
		if aid, err := source.RepoID(a.URL); err == nil && aid == id {
			return true
		}
	}
	return false
}

// newSubmenuScreen builds the per-plugin command submenu as a reusable picker.
// Each row carries its own Pick, so new global commands are added as rows here.
func newSubmenuScreen(g globalItem, sh *core.Shared) *components.PickerScreen {
	c := appctx.Of(sh)
	items := []list.Item{}

	if !g.InProject(c.ProjectAddons) {
		items = append(items, components.Item{
			Name: "⬇ Import to Project",
			Desc: "add this plugin to the project manifest",
			Pick: func(sh *core.Shared) core.Action { return importToProject(sh, g) },
		})
	}
	items = append(items, components.Item{
		Name: "⛃ Archive",
		Desc: "browse the repo's versions and save a local copy",
		Pick: func(sh *core.Shared) core.Action {
			return core.Push(pck.BrowseRepo(g.url, pck.BrowseOpts{
				Source:       pck.SourceAll,
				IncludeHEAD:  true,
				Endpoint:     pck.ArchiveEndpoint,
				MarkArchived: true,
			}))
		},
	})
	items = append(items, components.Item{
		Name: "✎ Edit Manifest",
		Desc: "edit this plugin's global entry (url, path, version, tag, clone)",
		Pick: func(sh *core.Shared) core.Action {
			gp, err := addon.GlobalListPath()
			if err != nil {
				return core.SetStatusAndLog("error: " + err.Error())
			}
			a := addon.Addon{Name: g.name, URL: g.url, Path: g.path, Version: g.version, Tag: g.tag, Clone: g.clone}
			return core.Push(editmanifest.New(gp, a, appctx.GlobalDirty{}, true))
		},
	})
	items = append(items, components.Item{
		Name: "✗ Remove",
		Desc: "remove from the global list (and optionally its archive)",
		Pick: func(sh *core.Shared) core.Action { return core.Push(newRemoveConfirm(g)) },
	})

	// PopStop: this submenu is the per-plugin command hub, so the archive sub-flow
	// returns here (PopTo) after it finishes.
	return components.NewPicker(items, components.PickerOpts{
		// Crumb: "Plugin", // looks better with name
		Title:   g.name,
		PopStop: true,
	})
}

// importToProject copies the global entry into the project manifest, then broadcasts
// ProjectDirty (Focus false, so the Project list reloads silently without leaving the
// Global tab) and pops the submenu back to the Global list — handy for importing several.
func importToProject(sh *core.Shared, g globalItem) core.Action {
	if err := addon.AddEntry(appctx.Of(sh).ManifestPath, g.name, g.url, g.path); err != nil {
		return core.Seq(
			core.SetStatusAndLog("error: "+err.Error()),
			core.ResetToRoot(),
		)
	}
	return core.Seq(
		core.SetStatus("imported "+g.name),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.Pop(),
	)
}

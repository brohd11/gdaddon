package global

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/editmanifest"
	pck "github.com/brohd11/gdaddon/internal/tui/flows/packages"
	"github.com/brohd11/gdaddon/internal/tui/sysopen"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/list"
)

// InProject reports whether this global plugin's repo is already present in the
// given project addon list (matched by source.RepoID).
func (g globalItem) InProject(addons []addon.Addon) bool {
	_, ok := addon.FindByRepo(addons, g.url)
	return ok
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
		Name: "{ } Open Source",
		Desc: "open the source URL in your browser",
		Pick: func(sh *core.Shared) core.Action { return sysopen.URL(g.url) },
	})
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
				return core.StatusErr(err)
			}
			a := g.entry()
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
		Title:   g.label(),
		PopStop: true,
	})
}

// importToProject copies the entry into the project manifest, broadcasts ProjectDirty (the
// project list reloads without switching tabs) and pops back, so several can be imported.
func importToProject(sh *core.Shared, g globalItem) core.Action {
	a := g.entry()
	if err := addon.AddEntryFull(appctx.Of(sh).ManifestPath, a); err != nil {
		return core.SeqErr(err, core.ResetToRoot())
	}
	return core.Seq(
		core.SetStatus("imported "+g.label()),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.Pop(),
	)
}

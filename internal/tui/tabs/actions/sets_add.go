package actions

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	pck "gdaddon/internal/tui/flows/packages"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newSetAddEntryPicker lists the global plugins not already in the set as
// candidates; selecting one opens its Add Plugin / Add Version submenu.
func newSetAddEntryPicker(setName string) core.Screen {
	setPath, _ := addon.SetPath(setName)
	var items []list.Item
	if gp, err := addon.GlobalListPath(); err == nil {
		if globals, err := addon.Parse(gp); err == nil {
			for _, g := range globals {
				g := g
				if inSet(setPath, g.URL) {
					continue
				}
				items = append(items, components.Item{
					Name: g.Name,
					Desc: g.URL,
					Pick: func(sh *core.Shared) core.Action { return core.Push(newSetPluginSubmenu(setName, setPath, g)) },
				})
			}
		}
	}
	if len(items) == 0 {
		items = append(items, components.Item{
			Name: "(no plugins to add)",
			Desc: "every global plugin is already in this set (or none exist)",
		})
	}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Add entry", Title: setName})
}

// newSetPluginSubmenu is a chosen global plugin's add menu: "Add Plugin" (url only,
// no version) and "Add Version" (browse the repo's upstream versions and pin one).
// Candidates are pre-filtered to non-members, so Add Plugin always applies.
func newSetPluginSubmenu(setName, setPath string, g addon.Addon) *components.PickerScreen {
	items := []list.Item{
		components.Item{
			Name: "+ Add Plugin",
			Desc: "add to the set (url only, no version)",
			Pick: func(sh *core.Shared) core.Action {
				if err := addon.AddEntry(setPath, g.Name, addon.NormalizeRepoURL(g.URL), g.Path); err != nil {
					return core.SetStatusAndLog("error: " + err.Error())
				}
				return core.Seq(
					core.SetStatusAndLog("added "+g.Name+" to "+setName),
					core.PropagateAll(appctx.SetsDirty{}),
					core.PopTo(),
				)
			},
		},
		setAddVersionItem(setName, setPath, g.Name, g.URL, g.Path),
	}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Plugins", Title: g.Name})
}

// setAddVersionItem is the shared "Add Version" row: it browses url's upstream
// versions (the packages flow) and pins the chosen one into the set via
// setVersionEndpoint. Reused by the add-entry candidate submenu and the per-member
// submenu.
func setAddVersionItem(setName, setPath, name, url, path string) components.Item {
	return components.Item{
		Name: "◷ Add Version",
		Desc: "browse the repo's versions and pin one",
		Pick: func(sh *core.Shared) core.Action {
			return core.Push(pck.BrowseRepo(url, pck.BrowseOpts{
				Source:      pck.SourceRemote,
				IncludeHEAD: true,
				Endpoint:    setVersionEndpoint(setName, setPath, name, path),
			}))
		},
	}
}

// setVersionEndpoint is the packages-flow leaf for Add Version: it confirms the
// chosen version, then pins it (url = the asset's download URL, version = the tag)
// into the set and returns to the set's command hub (PopTo).
func setVersionEndpoint(setName, setPath, pluginName, path string) pck.Endpoint {
	return func(sel pck.Selection) core.Screen {
		items := []list.Item{
			components.Item{
				Name: "+ Add " + sel.Tag,
				Desc: "pin this version in " + setName + " (replaces any existing entry)",
				Pick: func(sh *core.Shared) core.Action {
					// Record the release tag for a real release (a branch pin has no tag,
					// matching the project install's branch-tag rule).
					validTag := ""
					if !sel.Branch && sel.Tag != "" {
						validTag = sel.Tag
					}
					if err := addon.UpsertEntry(setPath, pluginName, sel.Asset.URL, path, sel.Tag, validTag); err != nil {
						return core.SetStatusAndLog("error: " + err.Error())
					}
					return core.Seq(
						core.SetStatus("added "+pluginName+" "+sel.Tag+" to "+setName),
						core.PropagateAll(appctx.SetsDirty{}),
						core.PopTo(),
					)
				},
			},
		}
		return components.NewPicker(items, components.PickerOpts{Crumb: "Add Entry", Title: pluginName + " — " + sel.Tag})
	}
}

// inSet reports whether the set already has an entry for url's repo (matched by
// source.RepoID, so .git vs release-zip forms collapse).
func inSet(setPath, url string) bool {
	id, err := source.RepoID(url)
	if err != nil {
		return false
	}
	entries, err := addon.Parse(setPath)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if eid, err := source.RepoID(e.URL); err == nil && eid == id {
			return true
		}
	}
	return false
}

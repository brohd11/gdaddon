package actions

import (
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/editmanifest"
	pck "gdaddon/internal/tui/flows/packages"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// ---------- add entry ----------

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

// ---------- plugins (the set's members) ----------

// newSetPluginsPicker lists the set's current members (name + pinned version);
// selecting one opens its per-member submenu (Add Version / Remove plugin).
func newSetPluginsPicker(setName string) core.Screen {
	setPath, _ := addon.SetPath(setName)
	var items []list.Item
	if entries, err := addon.Parse(setPath); err == nil {
		for _, e := range entries {
			e := e
			desc := e.Version
			if desc == "" {
				desc = "(no version pinned)"
			}
			items = append(items, components.Item{
				Name: e.Name,
				Desc: desc,
				Pick: func(sh *core.Shared) core.Action { return core.Push(newSetEntrySubmenu(setName, setPath, e)) },
			})
		}
	}
	if len(items) == 0 {
		items = append(items, components.Item{Name: "(set is empty)", Desc: "add plugins via Add entry"})
	}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Plugins", Title: setName})
}

// newSetEntrySubmenu is a set member's command menu: re-pin a version (Add Version)
// or drop it from the set (Remove plugin). Both return to the set's command hub.
func newSetEntrySubmenu(setName, setPath string, e addon.Addon) *components.PickerScreen {
	items := []list.Item{
		setAddVersionItem(setName, setPath, e.Name, e.URL, e.Path),
		components.Item{
			Name: "✎ Edit Manifest",
			Desc: "edit this set entry (url, path, version, tag, clone)",
			Pick: func(sh *core.Shared) core.Action {
				return core.Push(editmanifest.New(setPath, e, appctx.SetsDirty{}, false))
			},
		},
		components.Item{
			Name: "✗ Remove plugin",
			Desc: "remove this plugin from the set",
			Pick: func(sh *core.Shared) core.Action {
				return core.Push(newRemovePluginConf(setName, setPath, e))
			},
		},
	}
	return components.NewPicker(items, components.PickerOpts{Title: e.Name})
}

func newRemovePluginConf(setName, setPath string, e addon.Addon) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Text: fmt.Sprintf("Remove %s from %s?", e.Name, setName),
		OnYesLamda: func(sh *core.Shared) core.Action {
			if err := addon.RemoveEntry(setPath, e.Name); err != nil {
				return core.SetStatusAndLog("error: " + err.Error())
			}
			return core.Seq(
				core.SetStatus("removed "+e.Name+" from "+setName),
				core.PropagateAll(appctx.SetsDirty{}),
				core.PopTo(), // pop to set menu and push refreshed plugins menu
				core.Push(newSetPluginsPicker(setName)),
			)
		},
	})
}

// ---------- import ----------

// importSetToProject adds every entry in the set to the project manifest (deduped by
// repo id, carrying any pinned version), then shows the Project tab reloaded.
func importSetToProject(sh *core.Shared, setName string) core.Action {
	c := appctx.Of(sh)
	if c.ManifestPath == "" {
		return core.SetStatusAndLog("no project manifest — create one first (Actions → Create manifest)")
	}
	setPath, err := addon.SetPath(setName)
	if err != nil {
		return core.SetStatusAndLog("error: " + err.Error())
	}
	entries, err := addon.Parse(setPath)
	if err != nil {
		return core.SetStatusAndLog("error: " + err.Error())
	}
	added, skipped := 0, 0
	for _, e := range entries {
		if err := addon.AddEntryWithVersion(c.ManifestPath, e.Name, e.URL, e.Path, e.Version, e.Tag); err != nil {
			skipped++
			continue
		}
		added++
	}
	// Acknowledge with a popup over the Set submenu; dismissing it reloads the
	// Project tab and jumps there (ShowTab unwinds the stack, discarding the popup).
	return core.Push(newImportDonePopup(setName, added, skipped))
}

// newImportDonePopup is the "job done" acknowledgement shown after an import: a small
// box summarizing the result; pressing done reloads and shows the Project tab.
func newImportDonePopup(setName string, added, skipped int) *components.DialogScreen {
	return &components.DialogScreen{
		Overlay: true, // a centered modal over the Set submenu
		Title:   "Import complete",
		Render: func(*core.Shared) string {
			return fmt.Sprintf("✓ %s\n\n%d added, %d skipped", setName, added, skipped)
		},
		OnYes: func(*core.Shared) core.Action {
			return core.Seq(
				core.PropagateAll(appctx.ProjectDirty{}),
				core.ShowTab(appctx.TitleProject),
			)
		},
		Help: components.DefaultPopupHelp,
	}
}

package actions

import (
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/editmanifest"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

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
	lockName, lockDesc := "🔒 Lock", "pin this version — stop update alerts"
	if e.IsLocked() {
		lockName, lockDesc = "🔓 Unlock", "resume update checks"
	}
	items := []list.Item{
		setAddVersionItem(setName, setPath, e.Name, e.URL, e.Path),
		components.Item{
			Name: lockName,
			Desc: lockDesc,
			Pick: func(sh *core.Shared) core.Action { return toggleSetLock(setName, setPath, e) },
		},
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

// toggleSetLock flips the set entry's lock flag, then re-renders the member submenu
// so its Lock/Unlock row reflects the new state. Mirrors the project tab's toggleLock.
func toggleSetLock(setName, setPath string, e addon.Addon) core.Action {
	newLock := !e.Lock
	if err := addon.SetLock(setPath, e.Name, newLock); err != nil {
		return core.SetStatusAndLog("error: " + err.Error())
	}
	e.Lock = newLock
	verb := "locked"
	if !newLock {
		verb = "unlocked"
	}
	return core.Seq(
		core.SetStatus(verb+" "+e.Name+" in "+setName),
		core.PropagateAll(appctx.SetsDirty{}),
		core.Replace(newSetEntrySubmenu(setName, setPath, e)),
	)
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

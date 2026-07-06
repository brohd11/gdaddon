package sets

import (
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/editmanifest"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// setPluginsScreen lists a set's current members (name + pinned version); selecting one
// opens its per-member submenu. It is a pushed screen (Back pops it) but, unlike a plain
// picker, owns its Update so it can cycle the sort order (setsSortModes) and self-refresh
// on a SetsDirty broadcast after a member is added, pinned, or removed.
type setPluginsScreen struct {
	setName string
	setPath string
	list    list.Model
	sort    appctx.SortMode
}

var _ core.Filterer = (*setPluginsScreen)(nil)
var _ core.Receiver = (*setPluginsScreen)(nil)
var _ core.Crumber = (*setPluginsScreen)(nil)

func (s *setPluginsScreen) CrumbLabel(bool) string { return "Plugins" }

// newSetPluginsPicker builds the member-list screen for a set.
func newSetPluginsPicker(setName string) core.Screen {
	setPath, _ := addon.SetPath(setName)
	return &setPluginsScreen{
		setName: setName,
		setPath: setPath,
		list:    core.NewSelectList(setPluginRows(setName, setPath, appctx.SortAlpha), appctx.SortTitle(setName, appctx.SortAlpha)),
	}
}

// setPluginRows builds one row per set member (name + pinned version), ordered by mode;
// selecting a member opens its submenu. An empty set shows a placeholder.
func setPluginRows(setName, setPath string, mode appctx.SortMode) []list.Item {
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
	appctx.SortItemsByTitle(items, mode == appctx.SortReverse)
	return components.EnsurePlaceholder(items, "(set is empty)", "add plugins via Add entry")
}

func (s *setPluginsScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *setPluginsScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

// Update cycles the sort order, pops on Back (a pushed screen), and otherwise forwards
// to the shared root key-handling.
func (s *setPluginsScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	if km, ok := msg.(tea.KeyMsg); ok && !s.Filtering() {
		switch {
		case core.MatchKey(km.String(), appctx.AppKeys.Sort):
			appctx.CycleSort(&s.list, &s.sort, setsSortModes, s.setName,
				func(m appctx.SortMode) []list.Item { return setPluginRows(s.setName, s.setPath, m) })
			return s, core.Action{}
		case core.MatchKey(km.String(), core.Keys.Back):
			return s, core.Pop()
		}
	}
	return s, components.RootUpdate(sh, &s.list, msg)
}

func (s *setPluginsScreen) View(*core.Shared) string { return s.list.View() }
func (s *setPluginsScreen) HelpView(*core.Shared) string {
	return core.ShortHelp(s.list, core.HelpMinimal)
}

// Receive reloads the member list on a SetsDirty broadcast (after an add/pin/remove/edit),
// so returning to this screen shows the change without a rebuild-on-push.
func (s *setPluginsScreen) Receive(sh *core.Shared, payload any) core.Action {
	if _, ok := payload.(appctx.SetsDirty); ok {
		s.list.SetItems(setPluginRows(s.setName, s.setPath, s.sort))
	}
	return core.Action{}
}

func (s *setPluginsScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
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
	newLock, verb, err := appctx.LockToggle(setPath, e.Name, e.Lock)
	if err != nil {
		return core.StatusErr(err)
	}
	e.Lock = newLock
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
				return core.StatusErr(err)
			}
			return core.Seq(
				core.SetStatus("removed "+e.Name+" from "+setName),
				core.PropagateAll(appctx.SetsDirty{}), // the plugins list self-refreshes
				core.Pop(2),                            // drop this confirm + the member submenu
			)
		},
	})
}

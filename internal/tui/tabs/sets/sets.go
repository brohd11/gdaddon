// Package sets is the Sets tab: reusable groups of plugins stored as manifest-shaped
// YAML under ~/.gdaddon/sets. The root lists the saved sets; drilling into one manages
// its members (add/remove/pin) or imports the whole set into the project manifest.
package sets

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// setsTitle is the set list's base Title; the active sort mode is appended.
const setsTitle = "Sets"

// setsSortModes is the sort cycle used by both the set list and the per-set member
// list: name A→Z then Z→A (sets/members carry no install state, so no status grouping).
var setsSortModes = []appctx.SortMode{appctx.SortAlpha, appctx.SortReverse}

// SetsScreen is the Sets tab root: a "+ New set" row pinned at the top followed by one
// row per saved set under ~/.gdaddon/sets. It is a Receiver, so it reloads its list
// after a set is created, edited, or deleted (the SetsDirty broadcast).
type SetsScreen struct {
	list list.Model
	sort appctx.SortMode
}

var _ core.Filterer = (*SetsScreen)(nil)
var _ core.Receiver = (*SetsScreen)(nil)
var _ core.Crumber = (*SetsScreen)(nil)

// CrumbLabel anchors the breadcrumb at the Sets root.
func (s *SetsScreen) CrumbLabel(bool) string { return "Tab" }

func NewSetsScreen(sh *core.Shared) *SetsScreen {
	return &SetsScreen{list: core.NewSelectList(setListItems(appctx.SortAlpha), appctx.SortTitle(setsTitle, appctx.SortAlpha))}
}

// setListItems builds the Sets rows: a fixed "+ New set" row on top, then a
// self-dispatching row per saved set (its description is the plugin count), ordered by
// mode. Only the set rows are sorted — "+ New set" stays pinned first. Selecting a set
// opens its options submenu.
func setListItems(mode appctx.SortMode) []list.Item {
	names, _ := addon.ListSets()
	setRows := make([]list.Item, 0, len(names))
	for _, name := range names {
		name := name
		desc := "set"
		if p, err := addon.SetPath(name); err == nil {
			if addons, err := addon.Parse(p); err == nil {
				desc = fmt.Sprintf("%d plugins", len(addons))
			}
		}
		setRows = append(setRows, components.Item{
			Name: name,
			Desc: desc,
			Pick: func(sh *core.Shared) core.Action { return core.Push(newSetOptions(sh, name)) },
		})
	}
	appctx.SortItemsByTitle(setRows, mode == appctx.SortReverse)
	items := []list.Item{
		components.Item{
			Name: "+ New set",
			Desc: "create a new set under ~/.gdaddon/sets",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newSetForm(sh)) },
		},
	}
	return append(items, setRows...)
}

func (s *SetsScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *SetsScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *SetsScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	if k, ok := msg.(tea.KeyMsg); ok && !s.Filtering() && core.MatchKey(k.String(), appctx.AppKeys.Sort) {
		appctx.CycleSort(&s.list, &s.sort, setsSortModes, setsTitle,
			func(m appctx.SortMode) []list.Item { return setListItems(m) })
		return s, core.Action{}
	}
	return s, components.RootUpdate(sh, &s.list, msg)
}

func (s *SetsScreen) View(*core.Shared) string { return s.list.View() }
func (s *SetsScreen) HelpView(*core.Shared) string {
	return core.ShortHelp(s.list, core.HelpTabbed)
}

// Receive reloads the set list from disk on a SetsDirty broadcast (after a set is
// created/edited/deleted), so the tab reflects the change.
func (s *SetsScreen) Receive(sh *core.Shared, payload any) core.Action {
	if _, ok := payload.(appctx.SetsDirty); ok {
		s.list.SetItems(setListItems(s.sort))
	}
	return core.Action{}
}

func (s *SetsScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

// ---------- new set ----------

// newSetForm builds the New-set form: a name field plus, when a project manifest is
// loaded, a Seed toggle (Empty / Current project). On submit it creates
// <name>.yml under ~/.gdaddon/sets — verbatim-copying the project manifest when
// seeding — broadcasts SetsDirty so the list reloads, and pops back to it.
func newSetForm(sh *core.Shared) *components.FormScreen {
	manifestPath := appctx.Of(sh).ManifestPath
	nameF := components.NewTextField("name", "Name: ", "my_set")

	fields := []components.FormField{
		components.NewHeading("New set"),
		components.NewNote("Creates a set under ~/.gdaddon/sets; add plugins to it later."),
		components.NewSpacer(),
		nameF,
	}
	var seed *components.ToggleField
	if manifestPath != "" {
		seed = components.NewToggleField("seed", "Seed: ", []string{"Empty", "Current project"}, "|")
		fields = append(fields, components.NewSpacer(), seed)
	}

	help := []key.Binding{
		core.Hint("create", core.Keys.Select),
		core.Hint("cancel", core.Keys.Back),
	}
	if seed != nil {
		help = []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint("seed", core.Keys.Left, core.Keys.Right),
			core.Hint("create", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		}
	}

	return components.NewForm(components.FormOpts{
		Crumb:  "New Set",
		Fields: fields,
		Focus:  "name",
		Help:   help,
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			name := strings.TrimSpace(f.Value("name"))
			switch {
			case name == "":
				return core.Seq(core.SetStatusAndLog("set name is required"), core.Async(f.Focus("name")))
			case strings.ContainsAny(name, `/\`):
				return core.Seq(core.SetStatusAndLog("set name cannot contain path separators"), core.Async(f.Focus("name")))
			}
			from := ""
			if seed != nil && seed.Index() == 1 {
				from = manifestPath
			}
			if _, err := addon.CreateSetFrom(name, from); err != nil {
				return core.SeqErr(err, core.Async(f.Focus("name")))
			}
			status := "created set " + name
			if from != "" {
				status += " from current project"
			}
			return core.Seq(
				core.SetStatusAndLog(status),
				core.PropagateAll(appctx.SetsDirty{}),
				core.Pop(),
			)
		},
	})
}

// ---------- set options (a PopStop command hub) ----------

// newSetOptions is the per-set command submenu. It is a PopStop hub, so the deeper
// add-entry sub-flow returns here (PopTo) after committing. Import is offered only
// when a project manifest is loaded.
func newSetOptions(sh *core.Shared, setName string) *components.PickerScreen {
	items := []list.Item{
		components.Item{
			Name: "≣ Plugins",
			Desc: "manage the plugins in this set",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newSetPluginsPicker(setName)) },
		},
		components.Item{
			Name: "+ Add entry",
			Desc: "add a plugin from your global list",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newSetAddEntryPicker(setName)) },
		},
	}
	if appctx.Of(sh).ManifestPath != "" {
		items = append(items, components.Item{
			Name: "⬇ Import to Project",
			Desc: "add every plugin in this set to the project manifest",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newImportConfirm(setName)) },
		})
	}
	items = append(items, components.Item{
		Name: "✗ Delete set",
		Desc: "delete this set file",
		Pick: func(sh *core.Shared) core.Action { return core.Push(newSetDeleteConfirm(setName)) },
	})
	return components.NewPicker(items, components.PickerOpts{Crumb: "Set", Title: setName, PopStop: true})
}

// The set sub-flows — the member list (sets_members.go), add entry + version pinning
// (sets_add.go), and import-to-project (sets_import.go) — live alongside this file.

// ---------- delete ----------

// newSetDeleteConfirm confirms deleting the set file, then returns to the (reloaded)
// set list.
func newSetDeleteConfirm(setName string) *components.DialogScreen {
	return &components.DialogScreen{
		Crumb: "Delete",
		Render: func(sh *core.Shared) string {
			return sh.Box("Delete set: " + setName + "\n\n  Removes the set file. Installed plugins are not touched.")
		},
		OnYes: func(sh *core.Shared) core.Action {
			if err := addon.DeleteSet(setName); err != nil {
				return core.SeqErr(err, core.PopTo())
			}
			return core.Seq(
				core.SetStatusAndLog("deleted set "+setName),
				core.PropagateAll(appctx.SetsDirty{}),
				core.Pop(2), // drop this confirm + the set's options, back to the set list
			)
		},
		Help: []key.Binding{core.Hint("delete", core.Keys.Yes), core.Hint("cancel", core.Keys.No)},
	}
}

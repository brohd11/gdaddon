package actions

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

// SetListScreen is the Sets submenu (Actions ▸ Sets): a "+ New set" row at the top
// followed by one row per saved set under ~/.gdaddon/sets. It is a pushed screen
// (Back pops it) and a Receiver, so it reloads its list after a set is created,
// edited, or deleted (the SetsDirty broadcast).
type SetListScreen struct{ list list.Model }

var _ core.Filterer = (*SetListScreen)(nil)
var _ core.Receiver = (*SetListScreen)(nil)
var _ core.Crumber = (*ActionsScreen)(nil)

// CrumbLabel anchors the breadcrumb at the Actions root.
func (s *SetListScreen) CrumbLabel(bool) string { return "Sets" } // s.list.Title }

func newSetListScreen() *SetListScreen {
	return &SetListScreen{list: core.NewSelectList(setListItems(), "Sets")}
}

// setListItems builds the Sets submenu rows: "+ New set" plus a self-dispatching
// row per saved set (its description is the plugin count). Selecting a set opens
// its options submenu.
func setListItems() []list.Item {
	items := []list.Item{
		components.Item{
			Name: "+ New set",
			Desc: "create a new set under ~/.gdaddon/sets",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newSetForm(sh)) },
		},
	}
	names, _ := addon.ListSets()
	for _, name := range names {
		name := name
		desc := "set"
		if p, err := addon.SetPath(name); err == nil {
			if addons, err := addon.Parse(p); err == nil {
				desc = fmt.Sprintf("%d plugins", len(addons))
			}
		}
		items = append(items, components.Item{
			Name: name,
			Desc: desc,
			Pick: func(sh *core.Shared) core.Action { return core.Push(newSetOptions(sh, name)) },
		})
	}
	return items
}

func (s *SetListScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *SetListScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

// Update reuses the shared root key-handling (filter/select/list), adding only
// Back ▸ Pop since this is a pushed screen rather than a tab root.
func (s *SetListScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	if !s.Filtering() {
		if km, ok := msg.(tea.KeyMsg); ok && core.MatchKey(km.String(), core.Keys.Back) {
			return s, core.Pop()
		}
	}
	return s, components.RootUpdate(sh, &s.list, msg)
}

func (s *SetListScreen) View(*core.Shared) string { return s.list.View() }
func (s *SetListScreen) HelpView(*core.Shared) string {
	return core.ShortHelp(s.list, core.HelpMinimal)
}

// Receive reloads the set list from disk on a SetsDirty broadcast (after a set is
// created/edited/deleted), so the submenu reflects the change.
func (s *SetListScreen) Receive(sh *core.Shared, payload any) core.Action {
	if _, ok := payload.(appctx.SetsDirty); ok {
		s.list.SetItems(setListItems())
	}
	return core.Action{}
}

func (s *SetListScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
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
				return core.Seq(core.SetStatusAndLog("error: "+err.Error()), core.Async(f.Focus("name")))
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

// The set sub-flows — add entry, the member list, version pinning, remove, and
// import-to-project — live in sets_flows.go.

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
				return core.Seq(core.SetStatusAndLog("error: "+err.Error()), core.PopTo())
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

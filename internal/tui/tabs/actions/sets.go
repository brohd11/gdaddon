package actions

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	pck "gdaddon/internal/tui/flows/packages"

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

func (s *SetListScreen) View(*core.Shared) string     { return s.list.View() }
func (s *SetListScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpMinimal) }

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
		Crumb:  "New set",
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
				core.SetStatus(status),
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
			Pick: func(sh *core.Shared) core.Action { return importSetToProject(sh, setName) },
		})
	}
	items = append(items, components.Item{
		Name: "✗ Delete set",
		Desc: "delete this set file",
		Pick: func(sh *core.Shared) core.Action { return core.Push(newSetDeleteConfirm(setName)) },
	})
	return components.NewPicker(items, components.PickerOpts{Title: "Set: " + setName, PopStop: true})
}

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
	return components.NewPicker(items, components.PickerOpts{Title: setName + " — Add entry"})
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
				if err := addon.AddEntry(setPath, g.Name, addon.NormalizeRepoURL(g.URL), ""); err != nil {
					return core.SetStatusAndLog("error: " + err.Error())
				}
				return core.Seq(
					core.SetStatus("added "+g.Name+" to "+setName),
					core.PropagateAll(appctx.SetsDirty{}),
					core.PopTo(),
				)
			},
		},
		setAddVersionItem(setName, setPath, g.Name, g.URL),
	}
	return components.NewPicker(items, components.PickerOpts{Title: g.Name})
}

// setAddVersionItem is the shared "Add Version" row: it browses url's upstream
// versions (the packages flow) and pins the chosen one into the set via
// setVersionEndpoint. Reused by the add-entry candidate submenu and the per-member
// submenu.
func setAddVersionItem(setName, setPath, name, url string) components.Item {
	return components.Item{
		Name: "◷ Add Version",
		Desc: "browse the repo's versions and pin one",
		Pick: func(sh *core.Shared) core.Action {
			return core.Push(pck.BrowseRepo(url, pck.BrowseOpts{
				Source:      pck.SourceRemote,
				IncludeHEAD: true,
				Endpoint:    setVersionEndpoint(setName, setPath, name),
			}))
		},
	}
}

// setVersionEndpoint is the packages-flow leaf for Add Version: it confirms the
// chosen version, then pins it (url = the asset's download URL, version = the tag)
// into the set and returns to the set's command hub (PopTo).
func setVersionEndpoint(setName, setPath, pluginName string) pck.Endpoint {
	return func(sel pck.Selection) core.Screen {
		items := []list.Item{
			components.Item{
				Name: "+ Add " + sel.Tag,
				Desc: "pin this version in " + setName + " (replaces any existing entry)",
				Pick: func(sh *core.Shared) core.Action {
					if err := addon.UpsertEntry(setPath, pluginName, sel.Asset.URL, "", sel.Tag); err != nil {
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
		return components.NewPicker(items, components.PickerOpts{Title: pluginName + " — " + sel.Tag})
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
	return components.NewPicker(items, components.PickerOpts{Title: setName + " — Plugins"})
}

// newSetEntrySubmenu is a set member's command menu: re-pin a version (Add Version)
// or drop it from the set (Remove plugin). Both return to the set's command hub.
func newSetEntrySubmenu(setName, setPath string, e addon.Addon) *components.PickerScreen {
	items := []list.Item{
		setAddVersionItem(setName, setPath, e.Name, e.URL),
		components.Item{
			Name: "✗ Remove plugin",
			Desc: "remove this plugin from the set",
			Pick: func(sh *core.Shared) core.Action {
				if err := addon.RemoveEntry(setPath, e.Name); err != nil {
					return core.SetStatusAndLog("error: " + err.Error())
				}
				return core.Seq(
					core.SetStatus("removed "+e.Name+" from "+setName),
					core.PropagateAll(appctx.SetsDirty{}),
					core.PopTo(),
				)
			},
		},
	}
	return components.NewPicker(items, components.PickerOpts{Title: e.Name})
}

// ---------- import / delete ----------

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
		if err := addon.AddEntryWithVersion(c.ManifestPath, e.Name, e.URL, e.Path, e.Version, ""); err != nil {
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
func newImportDonePopup(setName string, added, skipped int) *components.PopupScreen {
	return &components.PopupScreen{
		Title: "Import complete",
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

// newSetDeleteConfirm confirms deleting the set file, then returns to the (reloaded)
// set list.
func newSetDeleteConfirm(setName string) *components.ConfirmScreen {
	return &components.ConfirmScreen{
		Crumb: "Delete set",
		Render: func(sh *core.Shared) string {
			return sh.Box("Delete set\n\n  " + setName + "\n\n  Removes the set file. Installed plugins are not touched.")
		},
		OnYes: func(sh *core.Shared) core.Action {
			if err := addon.DeleteSet(setName); err != nil {
				return core.Seq(core.SetStatusAndLog("error: "+err.Error()), core.PopTo())
			}
			return core.Seq(
				core.SetStatus("deleted set "+setName),
				core.PropagateAll(appctx.SetsDirty{}),
				core.Pop(2), // drop this confirm + the set's options, back to the set list
			)
		},
		Help: []key.Binding{core.Hint("delete", core.Keys.Yes), core.Hint("cancel", core.Keys.No)},
	}
}

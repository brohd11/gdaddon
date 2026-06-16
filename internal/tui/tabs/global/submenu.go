package global

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// submenuKind identifies a command in a global plugin's submenu.
type submenuKind int

const (
	subImportToProject submenuKind = iota
)

// submenuItem is one command row in a global plugin's submenu.
type submenuItem struct {
	title string
	desc  string
	kind  submenuKind
}

func (i submenuItem) Title() string       { return i.title }
func (i submenuItem) FilterValue() string { return i.title }
func (i submenuItem) Description() string { return i.desc }

// newSubmenuScreen builds the per-plugin command submenu as a reusable picker.
// Today the only command is Import to Project; new global commands are added as
// rows here.
func newSubmenuScreen(g globalItem) *components.PickerScreen {
	items := []list.Item{
		submenuItem{title: "⬇ Import to Project", desc: "add this plugin to the project manifest", kind: subImportToProject},
	}
	return components.NewPicker(items, components.PickerOpts{
		Title: g.name,
		OnSelect: func(sh *core.Shared, it list.Item) tea.Cmd {
			cmd, ok := it.(submenuItem)
			if !ok {
				return nil
			}
			switch cmd.kind {
			case subImportToProject:
				return importToProject(sh, g)
			}
			return nil
		},
	})
}

// importToProject copies the global entry into the project manifest, then
// refreshes the Project tab with the new row (reloadCmd → MsgRootRefresh).
func importToProject(sh *core.Shared, g globalItem) tea.Cmd {
	if err := addon.AddEntry(sh.ManifestPath, g.name, g.url, g.path); err != nil {
		sh.StatusMsg = "error: " + err.Error()
		return core.ResetToRoot()
	}
	return reloadCmd(sh, "imported "+g.name)
}

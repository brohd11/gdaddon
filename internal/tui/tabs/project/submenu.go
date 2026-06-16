package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// menuKind identifies a command in an addon's top-level submenu.
type menuKind int

const (
	menuInstall menuKind = iota
	menuArchive
	menuRemove
)

// menuItem is one command row in an addon's submenu.
type menuItem struct {
	title string
	desc  string
	kind  menuKind
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) FilterValue() string { return i.title }
func (i menuItem) Description() string { return i.desc }

// newSubmenuScreen builds the per-addon command submenu (the screen reached by
// pressing enter on an addon row). Install opens the existing version-fetch flow;
// Archive (offered only when the addon is installed) opens the archive submenu;
// Remove is scaffolding for a future command and is inert for now.
func newSubmenuScreen(st addon.Status) *components.PickerScreen {
	a, local := st.Addon, st.LocalVersion

	items := []list.Item{
		menuItem{title: "↧ Install / update", desc: "pick a version, branch, or asset to install", kind: menuInstall},
	}
	if st.Present() {
		items = append(items, menuItem{title: "📦 Archive", desc: "save a local copy of this addon", kind: menuArchive})
	}
	items = append(items, menuItem{title: "✗ Remove", desc: "remove from the project (and optionally delete files)", kind: menuRemove})

	return components.NewPicker(items, components.PickerOpts{
		Title:   core.HeaderTitle(a.Name, local, ""),
		PopStop: true, // the per-addon command hub: sub-flows PopTo() back here
		OnSelect: func(sh *core.Shared, it list.Item) tea.Cmd {
			cmd, ok := it.(menuItem)
			if !ok {
				return nil
			}
			switch cmd.kind {
			case menuInstall:
				return core.Push(newReleasesLoading(a, local))
			case menuArchive:
				return core.Push(newArchiveSubmenu(st))
			case menuRemove:
				return core.Push(newRemoveConfirm(st))
			}
			return nil
		},
	})
}

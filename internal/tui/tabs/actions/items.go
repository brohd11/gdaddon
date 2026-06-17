package actions

import (
	"gdaddon/internal/tui/flows/newplugin"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// actionItems builds the Actions menu rows. Each row is a self-dispatching
// components.Item carrying its own Pick, so the tab root just runs the selected
// row's closure — no kind enum, no switch.
func actionItems() []list.Item {
	return []list.Item{
		components.Item{
			Name: "↧ Install / update all",
			Desc: "download everything per the manifest",
			Pick: func(sh *core.Shared) (tea.Msg, tea.Cmd) { return core.Push(newInstallAllTask()), nil },
		},
		components.Item{
			Name: "+ New Plugin",
			Desc: "add a plugin to the project or your global list",
			Pick: func(sh *core.Shared) (tea.Msg, tea.Cmd) { return core.Push(newplugin.NewNewPluginForm()), nil },
		},
		components.Item{
			Name: "◑ Theme",
			Desc: "change the color theme",
			Pick: func(sh *core.Shared) (tea.Msg, tea.Cmd) { return core.Push(newThemePicker()), nil },
		},
	}
}

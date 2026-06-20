package actions

import (
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	// "github.com/charmbracelet/bubbles/key"
)

func newInstallAllConfirm(sh *core.Shared) *components.ConfirmScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Text:  "Do you want to install all packages in project?",
		OnYes: core.Push(newInstallAllTask()),
	})
}

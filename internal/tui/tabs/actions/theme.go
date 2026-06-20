package actions

import (
	"gdaddon/internal/config"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newThemePicker lists the registered themes; selecting one applies it live via
// core.ApplyTheme, which repaints the chrome immediately and rebuilds every tab
// root. The picker stays open (its own list recolors when reopened). Scaffolding:
// this lives under Actions for now and can move to a config submenu later.
func newThemePicker() core.Screen {
	active := core.CurrentTheme()
	var items []list.Item
	initialIndex := 0
	for i, name := range core.ThemeNames() {
		name := name // capture per row
		if name == active {
			initialIndex = i
		}
		desc := ""
		if name == active {
			desc = "active"
		}
		items = append(items, components.Item{
			Name: name,
			Desc: desc,
			Pick: func(sh *core.Shared) core.Action {
				// Persist the choice so it loads at next startup; a write failure
				// must never block the live switch, so the error is dropped.
				_ = config.SaveTheme(name)
				return core.Seq(
					core.ApplyTheme(name),
					core.Replace(newThemePicker()),
				)
			},
		})
	}

	return components.NewPicker(items, components.PickerOpts{
		Crumb:        "Theme",
		InitialIndex: initialIndex,
	})
}

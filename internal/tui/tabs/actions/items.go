package actions

import (
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/newplugin"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// actionItems builds the Actions menu rows. Each row is a self-dispatching
// components.Item carrying its own Pick, so the tab root just runs the selected
// row's closure — no kind enum, no switch. The Create-manifest row is prepended only
// while no manifest is loaded (the bootstrap case); the row is rebuilt on a
// PathRefresh broadcast, so it disappears once a manifest exists.
func actionItems(sh *core.Shared) []list.Item {
	var items []list.Item
	if appctx.Of(sh).ManifestPath == "" {
		items = append(items, components.Item{
			Name: "✎ Create manifest",
			Desc: "create an addon_manifest.yml to track this project's plugins",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newCreateManifestForm(sh)) },
		})
	}
	return append(items,
		components.Item{
			Name: "↧ Install all",
			Desc: "download and install everything per the manifest",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newInstallAllConfirm(sh)) },
		},
		components.Item{
			Name: "⟳ Update all",
			Desc: "update installed addons to their latest release",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newUpdateAllLoading(sh)) },
		},
		components.Item{
			Name: "+ New Plugin",
			Desc: "add a plugin to the project or your global list",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newplugin.NewNewPluginForm()) },
		},
		components.Item{
			Name: "⛁ Sets",
			Desc: "save and import reusable groups of plugins",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newSetListScreen()) },
		},
		components.Item{
			Name: "◑ Theme",
			Desc: "change the color theme",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newThemePicker()) },
		},
	)
}

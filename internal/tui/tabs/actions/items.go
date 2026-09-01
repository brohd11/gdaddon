package actions

import (
	"runtime"

	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/docs"
	gitflow "github.com/brohd11/gdaddon/internal/tui/flows/git"
	"github.com/brohd11/gdaddon/internal/tui/flows/newplugin"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/list"
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
			Name: "✎ Create Manifest",
			Desc: "create an addon_manifest.yml to track this project's plugins",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newCreateManifestForm(sh)) },
		})
	}
	items = append(items,
		components.Item{
			Name: "↧ Install/Update All",
			Desc: "install or update everything in the manifest",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newInstallUpdatePicker(sh)) },
		},
		components.Item{
			Name: "+ New Plugin",
			Desc: "add a plugin to the project or your global list",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newplugin.NewNewPluginForm()) },
		},
		components.Item{
			Name: "⌖ Paths",
			Desc: "open path in the file manager",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newPathsPicker(sh)) },
		},
	)
	if appctx.Of(sh).ProjectRoot != "" {
		items = append(items, components.Item{
			Name: "⌕ Scan installed",
			Desc: "find installed plugins not in the manifest and track them",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newScanPicker(sh)) },
		})
		items = append(items, components.Item{
			Name: "⎇ Git",
			Desc: "fetch, pull, or push every git checkout in the project",
			Pick: func(sh *core.Shared) core.Action { return core.Push(gitflow.AllRepos(sh)) },
		})
	}
	// macOS quarantines compiled plugins' native binaries; offer a manual clear.
	if runtime.GOOS == "darwin" && appctx.Of(sh).ProjectRoot != "" {
		items = append(items, components.Item{
			Name: "⚿ Dequarantine Addons",
			Desc: "clear macOS quarantine from addons folder",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newDequarantineConfirm(sh)) },
		})
	}

	items = append(items, components.Item{
		Name: "◑ Theme",
		Desc: "change the color theme",
		Pick: func(sh *core.Shared) core.Action { return core.Push(components.ThemePicker()) },
	},
	)

	// The standard docs row (shared with the bubblestack Actions menu); absent if
	// the pages didn't compile into the build.
	if docsRow, ok := components.DocsItem(docs.Pages()); ok {
		items = append(items, docsRow)
	}

	items = append(items, components.Item{
		Name: "⟲ Update gdaddon",
		Desc: "check for a newer gdaddon release and install it",
		Pick: func(sh *core.Shared) core.Action { return core.Push(newSelfUpdateLoading(sh)) },
	})

	items = append(items, components.Item{
		Name: "⟳ Refresh",
		Desc: "manually refresh lists",
		Pick: func(sh *core.Shared) core.Action { return appctx.RefreshAll() },
	})

	return items
}

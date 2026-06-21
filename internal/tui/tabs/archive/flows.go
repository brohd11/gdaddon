package archive

import (
	"fmt"

	arch "gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	pck "gdaddon/internal/tui/flows/packages"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
)

var removeConfirmHelp = []key.Binding{
	core.Hint("remove", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

// newPackageSubmenu is the per-package command menu (a packages.Endpoint). Today it
// offers only Remove (keyed off the asset's local path); it stays a submenu so future
// archive actions slot in as more rows.
func newPackageSubmenu(sel pck.Selection) core.Screen {
	items := []list.Item{
		components.Item{
			Name: "✗ Remove from archive",
			Desc: "delete this archived package",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newRemoveConfirm(sel.RepoID, sel.Asset)) },
		},
	}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Package", Title: sel.RepoID + " - " + sel.Asset.Name})
}

// newRemoveConfirm confirms deleting one archived package, then refreshes the tab.
func newRemoveConfirm(repoID string, asset source.Asset) *components.DialogScreen {
	return &components.DialogScreen{
		// Crumb: repoID + " — Remove",
		Crumb: "Remove",
		Render: func(sh *core.Shared) string {
			return sh.Box(fmt.Sprintf("Remove from archive\n\n  %s\n\n  %s", asset.Name, asset.URL))
		},
		OnYes: func(sh *core.Shared) core.Action {
			if err := arch.Remove(asset.URL); err != nil {
				return core.Seq(
					core.SetStatusAndLog("error: "+err.Error()),
					core.ResetToRoot(),
				)
			}
			return core.Seq(
				core.SetStatus("removed "+asset.Name),
				core.PropagateAll(appctx.ArchiveDirty{}),
				core.ShowTab(appctx.TitleArchive),
			)
		},
		Help: removeConfirmHelp,
	}
}

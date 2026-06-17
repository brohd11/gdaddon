package archive

import (
	"fmt"

	arch "gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

var removeConfirmHelp = []key.Binding{
	core.Hint("remove", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

// newVersionsPicker lists a repo's archived versions (newest first). A version
// with a single asset drops straight to its package submenu; multiple assets open
// an asset picker first (mirrors the project versions.go release rule).
func newVersionsPicker(repo arch.RepoArchive) *components.PickerScreen {
	items := make([]list.Item, 0, len(repo.Releases))
	for _, rel := range repo.Releases {
		rel := rel
		items = append(items, components.Item{
			Name: rel.Tag,
			Desc: fmt.Sprintf("%d asset(s)", len(rel.Assets)),
			Pick: func(sh *core.Shared) tea.Cmd {
				if len(rel.Assets) == 1 {
					return core.Push(newPackageSubmenu(repo.ID, rel.Assets[0]))
				}
				return core.Push(newAssetPicker(repo.ID, rel))
			},
		})
	}
	return components.NewPicker(items, components.PickerOpts{Title: repo.ID})
}

// newAssetPicker lists the assets of a multi-asset archived release; selecting one
// opens its package submenu.
func newAssetPicker(repoID string, rel source.Release) *components.PickerScreen {
	items := make([]list.Item, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		a := a
		items = append(items, components.Item{
			Name: a.Name,
			Pick: func(sh *core.Shared) tea.Cmd { return core.Push(newPackageSubmenu(repoID, a)) },
		})
	}
	return components.NewPicker(items, components.PickerOpts{Title: repoID + " — " + rel.Tag})
}

// newPackageSubmenu is the per-package command menu. Today it offers only Remove;
// it stays a submenu so future archive actions slot in as more rows.
func newPackageSubmenu(repoID string, asset source.Asset) *components.PickerScreen {
	items := []list.Item{
		components.Item{
			Name: "✗ Remove from archive",
			Desc: "delete this archived package",
			Pick: func(sh *core.Shared) tea.Cmd { return core.Push(newRemoveConfirm(repoID, asset)) },
		},
	}
	return components.NewPicker(items, components.PickerOpts{Title: repoID})
}

// newRemoveConfirm confirms deleting one archived package, then refreshes the tab.
func newRemoveConfirm(repoID string, asset source.Asset) *components.ConfirmScreen {
	return &components.ConfirmScreen{
		Crumb: core.RenderTitleBar(repoID + " — Remove"),
		Render: func(sh *core.Shared) string {
			return sh.Box(fmt.Sprintf("Remove from archive\n\n  %s\n\n  %s", asset.Name, asset.URL))
		},
		OnYes: func(sh *core.Shared) tea.Cmd {
			if err := arch.Remove(asset.URL); err != nil {
				sh.SetStatus("error: " + err.Error())
				return core.ResetToRoot()
			}
			return core.Refresh(appctx.Archive, true, "removed "+asset.Name)
		},
		Help: removeConfirmHelp,
	}
}

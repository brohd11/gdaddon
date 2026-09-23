package project

import (
	"fmt"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/tui/flows/packages"
	"github.com/brohd11/gdaddon/internal/tui/widgets"

	"charm.land/bubbles/v2/key"
)

// Confirm builders for the project tab (the install confirm is in confirm_install.go).

var confirmHelp = []key.Binding{
	core.Hint("confirm", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

// ---------- remove confirm ----------

// remove modes (also the vertical option order).
const (
	removeLocal        = iota // delete installed files only, keep manifest entry
	removeProject             // remove the manifest entry only
	removeProjectLocal        // also delete the installed files
)

// newRemoveConfirm chooses between removing the entry and also deleting its files (↑/↓,
// enter). A submodule's files belong to its parent, so it only removes the entry.
func newRemoveConfirm(st addon.Status) *components.DialogScreen {
	if st.Addon.IsSubmodule() {
		return &components.DialogScreen{
			Render: func(sh *core.Shared) string {
				path := st.Addon.Path
				if path == "" {
					path = "(none)"
				}
				return sh.Box(fmt.Sprintf("Remove submodule %s\n\n  path:  %s\n\n  removes the manifest entry only;\n  the parent repo still manages the files", st.Addon.Label(), path))
			},
			OnYes: func(sh *core.Shared) core.Action { return commitRemove(sh, st, removeProject) },
			Help:  confirmHelp,
		}
	}
	return widgets.NewToggleConfirm(widgets.ToggleConfirm{
		Crumb:  "Remove",
		Count:  3,
		Start:  removeLocal,
		Render: func(sh *core.Shared, mode int) string { return sh.Box(removeConfirmBody(sh, st, mode)) },
		OnPick: func(sh *core.Shared, mode int) core.Action { return commitRemove(sh, st, mode) },
		Help:   widgets.RemoveConfirmHelp,
	})
}

func removeConfirmBody(sh *core.Shared, st addon.Status, mode int) string {
	return widgets.RemoveConfirmBody(st.Addon.Label(), "path", st.Addon.Path, removeOptions(mode))
}

// removeOptions renders the two removal modes stacked vertically, the active one
// marked and highlighted (vertical analog of the New Plugin target toggle).
func removeOptions(mode int) string {
	return widgets.RenderChoices(mode, []widgets.ToggleOpt{
		{Label: "Local files", Desc: "delete installed files, keep the manifest entry"},
		{Label: "Project", Desc: "remove from the project manifest only"},
		{Label: "Project + local files", Desc: "also delete the installed files"},
	})
}

// ---------- archive confirm ----------

// buildArchiveConfirm hands the chosen asset to packages.NewArchiveConfirm; ok is false
// (with a status) when there is nothing to archive.
func buildArchiveConfirm(selected addon.Addon, local string, pick versionItem) (*components.DialogScreen, string, bool) {
	repoID, err := source.RepoID(selected.URL)
	if err != nil {
		return nil, "cannot archive: " + err.Error(), false
	}
	return packages.NewArchiveConfirm(selected.Label(), repoID, pick.tag, []source.Asset{pick.asset})
}

package project

import (
	"fmt"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/flows/packages"
	"gdaddon/internal/tui/widgets"

	"github.com/charmbracelet/bubbles/key"
)

// The confirm box mechanism lives in components.ConfirmScreen; the builders below
// supply its crumb/render/onYes closures for each feature. The install confirm (with
// its source/clone-mode toggles) lives in confirm_install.go; the remove and archive
// confirms are below.

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

var removeConfirmHelp = []key.Binding{
	core.Hint("option", core.Keys.Up, core.Keys.Down),
	core.Hint("remove", core.Keys.Select),
	core.Hint("cancel", core.Keys.Back),
}

// newRemoveConfirm builds the project Remove confirm: a vertical selector between
// removing just the manifest entry or that plus the installed files. ↑/↓ move the
// selection (via the confirm's OnKey), enter commits the chosen mode.
func newRemoveConfirm(st addon.Status) *components.ConfirmScreen {
	mode := removeLocal // local copy the selector mutates
	return &components.ConfirmScreen{
		Crumb: "Remove",

		Render: func(sh *core.Shared) string { return sh.Box(removeConfirmBody(sh, st, mode)) },
		OnKey: func(sh *core.Shared, k string) core.Action {
			switch {
			case core.MatchKey(k, core.Keys.Up):
				if mode > removeLocal {
					mode--
				}
			case core.MatchKey(k, core.Keys.Down):
				if mode < removeProjectLocal {
					mode++
				}
			}
			return core.Action{}
		},
		OnYes: func(sh *core.Shared) core.Action { return commitRemove(sh, st, mode) },
		Help:  removeConfirmHelp,
	}
}

func removeConfirmBody(sh *core.Shared, st addon.Status, mode int) string {
	path := st.Addon.Path
	if path == "" {
		path = "(none)"
	}
	return fmt.Sprintf("Remove %s\n\n  path:  %s\n\n%s", st.Addon.Name, path, removeOptions(mode))
}

// removeOptions renders the two removal modes stacked vertically, the active one
// marked and highlighted (vertical analog of the New Plugin target toggle).
func removeOptions(mode int) string {
	return widgets.RenderToggle(mode, []widgets.ToggleOpt{
		{Label: "Local files", Desc: "delete installed files, keep the manifest entry"},
		{Label: "Project", Desc: "remove from the project manifest only"},
		{Label: "Project + local files", Desc: "also delete the installed files"},
	})
}

// ---------- archive confirm ----------

// buildArchiveConfirm resolves the repo id and hands the chosen version's asset to
// the shared packages.NewArchiveConfirm (which owns the confirm body, already-archived
// check, and download task). It returns ok=false (with a status line) when there is
// nothing to archive: an error or an already-archived selection.
func buildArchiveConfirm(selected addon.Addon, local string, pick versionItem) (*components.ConfirmScreen, string, bool) {
	repoID, err := source.RepoID(selected.URL)
	if err != nil {
		return nil, "cannot archive: " + err.Error(), false
	}
	return packages.NewArchiveConfirm(selected.Name, repoID, pick.tag, []source.Asset{pick.asset})
}

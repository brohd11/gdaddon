package global

import (
	"fmt"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"gdaddon/internal/tui/widgets"

	"github.com/charmbracelet/bubbles/key"
)

// remove modes (also the vertical option order).
const (
	removeGlobal        = iota // remove from the global list only
	removeGlobalArchive        // also delete the archived packages for the repo
)

var removeConfirmHelp = []key.Binding{
	core.Hint("option", core.Keys.Up, core.Keys.Down),
	core.Hint("remove", core.Keys.Select),
	core.Hint("cancel", core.Keys.Back),
}

// newRemoveConfirm builds the global Remove confirm: a vertical selector between
// removing just the global-list entry or that plus the repo's archived packages.
// ↑/↓ move the selection (via the confirm's OnKey), enter commits the chosen mode.
func newRemoveConfirm(g globalItem) *components.DialogScreen {
	return widgets.NewToggleConfirm(widgets.ToggleConfirm{
		Crumb:  g.name + " — Remove",
		Count:  2,
		Start:  removeGlobal, // default = non-destructive
		Render: func(sh *core.Shared, mode int) string { return sh.Box(removeConfirmBody(sh, g, mode)) },
		OnPick: func(sh *core.Shared, mode int) core.Action { return commitRemove(sh, g, mode) },
		Help:   removeConfirmHelp,
	})
}

func removeConfirmBody(sh *core.Shared, g globalItem, mode int) string {
	url := g.url
	if url == "" {
		url = "(none)"
	}
	return fmt.Sprintf("Remove %s\n\n  url:  %s\n\n%s", g.name, url, removeOptions(mode))
}

// removeOptions renders the two removal modes stacked vertically, the active one
// marked and highlighted.
func removeOptions(mode int) string {
	return widgets.RenderToggle(mode, []widgets.ToggleOpt{
		{Label: "Global", Desc: "remove from the global list only"},
		{Label: "Global + archive", Desc: "also delete the archived packages"},
	})
}

package global

import (
	"fmt"
	"strings"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
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
func newRemoveConfirm(g globalItem) *components.ConfirmScreen {
	mode := removeGlobal // local copy the selector mutates; default = non-destructive
	return &components.ConfirmScreen{
		Crumb:  g.name + " — Remove",
		Render: func(sh *core.Shared) string { return sh.Box(removeConfirmBody(sh, g, mode)) },
		OnKey: func(sh *core.Shared, k string) core.Action {
			switch {
			case core.MatchKey(k, core.Keys.Up):
				if mode > removeGlobal {
					mode--
				}
			case core.MatchKey(k, core.Keys.Down):
				if mode < removeGlobalArchive {
					mode++
				}
			}
			return core.Action{}
		},
		OnYes: func(sh *core.Shared) core.Action { return commitRemove(sh, g, mode) },
		Help:  removeConfirmHelp,
	}
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
	active := lipgloss.NewStyle().Foreground(core.FocusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(core.MutedColor)
	opts := []struct{ label, desc string }{
		{"Global", "remove from the global list only"},
		{"Global + archive", "also delete the archived packages"},
	}
	lines := make([]string, len(opts))
	for i, o := range opts {
		text := o.label + " — " + o.desc
		if i == mode {
			lines[i] = "  ▸ " + active.Render(text)
		} else {
			lines[i] = "    " + dim.Render(text)
		}
	}
	return strings.Join(lines, "\n")
}

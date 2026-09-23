package widgets

import (
	"fmt"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
)

// RemoveConfirmHelp is the hint row of every Remove toggle-confirm (options move with
// ↑/↓, not yes/no).
var RemoveConfirmHelp = []key.Binding{
	core.Hint("option", core.Keys.Up, core.Keys.Down),
	core.Hint("remove", core.Keys.Select),
	core.Hint("cancel", core.Keys.Back),
}

// RemoveConfirmBody renders a Remove box: what is removed, one detail line ("(none)" when
// empty) and the options.
func RemoveConfirmBody(name, label, value, options string) string {
	if value == "" {
		value = "(none)"
	}
	return fmt.Sprintf("Remove %s\n\n  %s:  %s\n\n%s", name, label, value, options)
}

// ToggleConfirm configures a confirm with a vertical option selector (↑/↓, enter).
type ToggleConfirm struct {
	Crumb  string
	Count  int                                    // number of options (clamp upper bound)
	Start  int                                    // initial selected index
	Render func(sh *core.Shared, mode int) string // full box body; caller calls sh.Box + RenderChoices
	OnPick func(sh *core.Shared, mode int) core.Action
	Help   []key.Binding
}

// NewToggleConfirm builds a DialogScreen whose ↑/↓ move an index within [0, Count-1];
// Render draws it and OnPick commits.
func NewToggleConfirm(tc ToggleConfirm) *components.DialogScreen {
	mode := tc.Start
	return &components.DialogScreen{
		Crumb:  tc.Crumb,
		Render: func(sh *core.Shared) string { return tc.Render(sh, mode) },
		OnKey: func(sh *core.Shared, k string) core.Action {
			switch {
			case core.MatchKey(k, core.Keys.Up):
				if mode > 0 {
					mode--
				}
			case core.MatchKey(k, core.Keys.Down):
				if mode < tc.Count-1 {
					mode++
				}
			}
			return core.Action{}
		},
		OnYes: func(sh *core.Shared) core.Action { return tc.OnPick(sh, mode) },
		Help:  tc.Help,
	}
}

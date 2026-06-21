package widgets

import (
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
)

// ToggleConfirm configures a confirm box with a vertical option selector: ↑/↓ move a
// selected index, the box redraws for it, and enter commits the chosen one. It carries
// no domain type — Render and OnPick close over whatever the caller needs.
type ToggleConfirm struct {
	Crumb  string
	Count  int                                    // number of options (clamp upper bound)
	Start  int                                    // initial selected index
	Render func(sh *core.Shared, mode int) string // full box body; caller calls sh.Box + RenderToggle
	OnPick func(sh *core.Shared, mode int) core.Action
	Help   []key.Binding
}

// NewToggleConfirm wires a components.DialogScreen whose ↑/↓ move a selected index
// within [0, Count-1] (no wrap); Render draws the box for the current index and OnPick
// commits it. It owns only the selector state + clamp, so every site keeps full control
// of its rendered body and commit action.
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

// Package widgets holds render helpers shared by sibling tabs, which cannot import each
// other.
package widgets

import (
	"strings"

	"github.com/brohd11/bubblestack/core"
)

// ToggleOpt is one row of a vertical option selector: a short label and a one-line
// description.
type ToggleOpt struct {
	Label string
	Desc  string
}

// RenderChoices stacks opts as "label — desc" rows, marking sel with "▸" and the accent,
// the rest muted.
func RenderChoices(sel int, opts []ToggleOpt) string {
	active := core.AccentStyle()
	dim := core.MutedStyle()
	lines := make([]string, len(opts))
	for i, o := range opts {
		text := o.Label + " — " + o.Desc
		if i == sel {
			lines[i] = "  ▸ " + active.Render(text)
		} else {
			lines[i] = "    " + dim.Render(text)
		}
	}
	return strings.Join(lines, "\n")
}

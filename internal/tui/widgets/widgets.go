// Package widgets holds small, domain-agnostic render helpers shared by more than
// one tab. Tabs can't import each other (project/global/archive are siblings), so a
// rendering snippet used by several of them lives here, one layer down. It names no
// gdaddon domain type and depends only on bubblestack/core for the shared palette.
package widgets

import (
	"strings"

	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/lipgloss"
)

// ToggleOpt is one row of a vertical option selector: a short label and a one-line
// description.
type ToggleOpt struct {
	Label string
	Desc  string
}

// RenderToggle stacks opts vertically as "label — desc" rows, marking index sel with
// a "▸" caret and the focused color while dimming the rest. It's the shared form of
// the install-source / clone-mode / remove-mode selectors shown inside confirm boxes.
func RenderToggle(sel int, opts []ToggleOpt) string {
	active := lipgloss.NewStyle().Foreground(core.FocusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(core.MutedColor)
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

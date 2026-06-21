package core

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Composite splices the foreground string fg onto the background bg at cell
// position (x, y) — column x, row y, both zero-based — and returns the combined
// frame. It is the primitive behind overlay/popup screens: the router renders the
// below-screen frame as bg, then Composite-s the popup box (fg) centered over it so
// the background stays visible around the box.
//
// Both strings are treated as grids of lines. For each fg line, the matching bg
// line is rebuilt as [bg cells before x] + [fg line] + [bg cells from x+width(fg)],
// so the box punches a hole exactly its own width — wider or narrower bg styling on
// either side is preserved. All measurement and slicing is display-cell aware
// (ansi.StringWidth / Truncate / TruncateLeft) so embedded ANSI styling in bg isn't
// corrupted, and a reset (\x1b[0m) brackets the fg segment so neither side's color
// bleeds across the seam. fg lines that fall outside bg's rows are dropped.
func Composite(bg, fg string, x, y int) string {
	if fg == "" {
		return bg
	}
	if x < 0 {
		x = 0
	}
	bgLines := strings.Split(bg, "\n")
	for i, fgLine := range strings.Split(fg, "\n") {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		bgLine := bgLines[row]

		// Left slice: the first x cells of the bg line, padded with spaces when the
		// bg line is shorter than x so the box still lands at column x.
		left := ansi.Truncate(bgLine, x, "")
		if gap := x - ansi.StringWidth(left); gap > 0 {
			left += strings.Repeat(" ", gap)
		}

		// Right slice: bg cells from x+width(fg) onward (everything the box doesn't cover).
		right := ansi.TruncateLeft(bgLine, x+ansi.StringWidth(fgLine), "")

		bgLines[row] = left + "\x1b[0m" + fgLine + "\x1b[0m" + right
	}
	return strings.Join(bgLines, "\n")
}

// popupStyle is the bordered popup box: a rounded border in the theme accent so the
// box reads as foreground over the (un-dimmed) screen behind it. rebuildStyles in
// shared.go does not own it (it has no palette-dependent state beyond the color read
// at render time), so it is built per call from the current FocusedColor.
func popupBox(width int) lipgloss.Style {
	s := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(FocusedColor)
	if width > 0 {
		s = s.Width(width)
	}
	return s
}

// PopupBox renders body inside a themed, bordered popup box, with an optional accent
// title line above it. width is the inner content width (0 ⇒ size to content). It is
// the default renderer for an overlay components.DialogScreen, mirroring (*Shared).Box
// for the layered case so popups follow the active theme.
func PopupBox(title, body string, width int) string {
	content := body
	if title != "" {
		head := lipgloss.NewStyle().Bold(true).Foreground(FocusedColor).Render(title)
		content = lipgloss.JoinVertical(lipgloss.Left, head, "", body)
	}
	return popupBox(width).Render(content)
}

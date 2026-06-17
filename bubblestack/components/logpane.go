package components

import (
	"strings"

	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogPane is the default core.Output: a scrollable log shown in a bordered box below
// the body. New lines auto-reveal it (Log), the Output key (o) toggles it, and while
// focused it scrolls. Supply it via bubblestack.Config.Output, or implement
// core.Output for a custom pane. It is context-agnostic — it names no domain type.
type LogPane struct {
	vp     viewport.Model
	logs   []string
	shown  bool
	width  int
	height int
}

// NewLogPane builds the default log output pane.
func NewLogPane() *LogPane { return &LogPane{vp: viewport.New(0, 0)} }

// Log appends a line and reveals the pane. This is the logging capability beyond
// core.Output that Shared.Log reaches by type assertion; the router never calls it.
func (p *LogPane) Log(line string) {
	p.logs = append(p.logs, line)
	p.shown = true
}

func (p *LogPane) Shown() bool { return p.shown }
func (p *LogPane) Toggle()     { p.shown = !p.shown }
func (p *LogPane) Hide()       { p.shown = false }

func (p *LogPane) Clear() {
	p.logs = nil
	p.shown = false
	p.vp.SetContent("")
}

// SetSize lays the viewport out for the terminal: full-width text and a fixed ~25%
// of the terminal height, so the log fills a stable region and scrolls past it
// rather than growing line by line. The router re-sets content here each cycle, so
// freshly appended lines appear without LogPane tracking the router.
func (p *LogPane) SetSize(termWidth, termHeight int) {
	p.width, p.height = termWidth, termHeight
	p.vp.Width = p.innerWidth()
	p.vp.Height = p.contentHeight()
	p.vp.SetContent(p.content())
}

// Height is the rows the pane occupies when shown (content + top/bottom border).
func (p *LogPane) Height() int {
	if !p.shown {
		return 0
	}
	return p.contentHeight() + 2
}

func (p *LogPane) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	p.vp, cmd = p.vp.Update(msg)
	return cmd
}

func (p *LogPane) GotoBottom() { p.vp.GotoBottom() }

// innerWidth is the text width inside the box (full width minus header-margin parity,
// side borders, and the 1-col padding on each side).
func (p *LogPane) innerWidth() int {
	w := p.width - 2 - 2 - 2
	if w < 10 {
		w = 10
	}
	return w
}

func (p *LogPane) contentHeight() int {
	n := p.height / 4
	if n < 3 {
		n = 3
	}
	return n
}

func (p *LogPane) content() string {
	style := core.LogStyle()
	var b strings.Builder
	for i, l := range p.logs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(style.Render(l))
	}
	return b.String()
}

// View draws the log inside a bordered box whose top edge is interrupted by an
// "Output" legend (plus a scroll hint while focused).
func (p *LogPane) View(focused bool) string {
	color := core.BorderColor
	label := "Output"
	if focused {
		color = core.FocusedColor
		label = "Output · ↑/↓ scroll · tab/esc back · o hide"
	}

	inner := p.innerWidth()
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderForeground(color).
		Padding(0, 1).
		Width(inner + 2) // inner text + the 1-col padding on each side
	content := box.Render(p.vp.View())

	// Hand-draw the top border so the legend can sit mid-line. The run between the
	// corners is the same width as the bottom border: inner + 2 (padding).
	legend := "─ " + label + " "
	fill := (inner + 2) - lipgloss.Width(legend)
	if fill < 0 {
		fill = 0
	}
	top := lipgloss.NewStyle().Foreground(color).
		Render("┌" + legend + strings.Repeat("─", fill) + "┐")

	return top + "\n" + content
}

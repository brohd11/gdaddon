package components

import (
	"strings"

	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// LogPane is the default core.Output: a scrollable log shown in a bordered box below
// the body. New lines auto-reveal it (Log), the Output key (o) toggles it, and while
// focused it scrolls. Supply it via bubblestack.Config.Output, or implement
// core.Output for a custom pane. It is context-agnostic — it names no domain type.
//
// It has two render modes over the same in-memory lines. Truncated (the default) gives
// each entry one row, which the viewport clips at the pane edge — fine for short lines,
// but a long one (a deep filesystem path, say) has a tail no amount of scrolling can
// reach. Wrapped (the Wrap key, w — see core.Wrapper) folds each entry across as many
// rows as it needs, bulleting it so the entry still reads as one unit.
type LogPane struct {
	vp     viewport.Model
	logs   []string
	shown  bool
	wrap   bool
	width  int
	height int
}

var _ core.Wrapper = (*LogPane)(nil)

// NewLogPane builds the default log output pane.
func NewLogPane() *LogPane { return &LogPane{vp: viewport.New(0, 0)} }

// Log appends a line and reveals the pane. This is the logging capability beyond
// core.Output that Shared.Log reaches by type assertion; the router never calls it.
func (p *LogPane) Log(line string, forceShow bool) {
	p.logs = append(p.logs, line)
	if forceShow {
		p.shown = forceShow
	}
}

func (p *LogPane) Shown() bool { return p.shown }
func (p *LogPane) Toggle()     { p.shown = !p.shown }
func (p *LogPane) Hide()       { p.shown = false }

// ToggleWrap switches between the truncated and wrapped render modes (core.Wrapper).
// The re-render is immediate rather than waiting on the next SetSize, and it lands at
// the bottom: the newly-wrapped tail is what a reader turns wrap on to see.
func (p *LogPane) ToggleWrap() {
	p.wrap = !p.wrap
	p.vp.SetContent(p.content())
	p.vp.GotoBottom()
}

func (p *LogPane) Wrapped() bool { return p.wrap }

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

// In wrapped mode an entry's first row carries the bullet and its continuations are
// indented under it, so entry boundaries survive folding.
const (
	logBullet = "- "
	logIndent = "  "
	// logBreaks are extra wrap points beyond whitespace, so a path folds at a
	// separator rather than mid-name ("-" is always a breakpoint in ansi.Wrap).
	logBreaks = "/_"
)

func (p *LogPane) content() string {
	style := core.LogStyle()
	var b strings.Builder
	for i, l := range p.logs {
		if i > 0 {
			b.WriteByte('\n')
		}
		if p.wrap {
			l = p.wrapEntry(l)
		}
		b.WriteString(style.Render(l))
	}
	return b.String()
}

// wrapEntry folds one entry to the pane width. ansi.Wrap (rather than Wordwrap) breaks
// word boundaries when it has to, so an unbroken token wider than the pane — the deep
// filesystem path that motivates the mode — is split instead of left overlong for the
// viewport to clip straight back off.
func (p *LogPane) wrapEntry(line string) string {
	w := p.innerWidth() - lipgloss.Width(logBullet)
	if w < 1 {
		w = 1
	}
	rows := strings.Split(ansi.Wrap(line, w, logBreaks), "\n")
	for i, r := range rows {
		if i == 0 {
			rows[i] = logBullet + r
			continue
		}
		rows[i] = logIndent + r
	}
	return strings.Join(rows, "\n")
}

// View draws the log inside a bordered box whose top edge is interrupted by an
// "Output" legend (plus a scroll hint while focused). Wrapped mode is advertised in the
// legend either way, so the render mode is visible without focusing the pane.
func (p *LogPane) View(focused bool) string {
	color := core.BorderColor
	label := "Output"
	if p.wrap {
		label = "Output [wrap]"
	}
	if focused {
		color = core.FocusedColor
		label = "Output · ↑/↓ scroll · tab/esc back · o hide · w wrap"
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

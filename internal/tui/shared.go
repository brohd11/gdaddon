package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"gdaddon/internal/addon"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// shared holds the cross-cutting state owned by the router and rendered as
// chrome around every screen: the project context box, terminal size, the
// spinner, the output/log pane (with its own focus mode), and a help model for
// rendering static help bars. A single instance is created in newShared and
// pointed at by the router; screens receive it as a method argument.
type shared struct {
	manifestPath string
	projectRoot  string
	manifestRel  string
	projectName  string
	hasProject   bool

	width  int
	height int

	spinner   spinner.Model
	output    viewport.Model
	help      help.Model // renders static (non-list) help bars
	logs      []string
	focus     focusArea
	statusMsg string

	events chan installEvent // the in-flight streaming task channel
}

func newShared(manifestPath, projectRoot string) *shared {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	rel, err := filepath.Rel(projectRoot, manifestPath)
	if err != nil {
		rel = manifestPath
	}
	name, exists := addon.ProjectName(projectRoot)

	h := help.New()
	h.Styles.ShortKey = h.Styles.ShortKey.Foreground(mutedColor)
	h.Styles.ShortDesc = h.Styles.ShortDesc.Foreground(mutedColor)
	h.Styles.ShortSeparator = h.Styles.ShortSeparator.Foreground(mutedColor)

	return &shared{
		manifestPath: manifestPath,
		projectRoot:  projectRoot,
		manifestRel:  rel,
		projectName:  name,
		hasProject:   exists,
		spinner:      sp,
		output:       viewport.New(0, 0),
		help:         h,
	}
}

var (
	// mutedColor is the secondary/muted gray (borders, labels, help, list
	// descriptions); logColor is brighter, near-white, for the output log text.
	mutedColor   = lipgloss.Color("247")
	logColor     = lipgloss.Color("252")
	borderColor  = lipgloss.Color("245")
	focusedColor = lipgloss.Color("212")

	statusStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(focusedColor)
	logStyle    = lipgloss.NewStyle().Foreground(logColor)
	boxStyle    = lipgloss.NewStyle().Margin(1, 2).Padding(1, 2).Border(lipgloss.RoundedBorder())
	headerStyle = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(borderColor)
	labelStyle  = lipgloss.NewStyle().Foreground(mutedColor)

	// listStyles are the default bubbles list styles, reused to render
	// breadcrumb/title bars and static help so they align with the real lists.
	listStyles = list.DefaultStyles()
)

// ---------- header ----------

// headerView renders the persistent context box shown on every screen.
func (s *shared) headerView() string {
	name := "No Project File"
	if s.hasProject {
		name = s.projectName
		if name == "" {
			name = "(unnamed project)"
		}
	}

	inner := s.width - 4 // minus border (2) and padding (2)
	if inner < 20 {
		inner = 20
	}
	valWidth := inner - 10 // minus the "Manifest: " label

	line := func(label, value string) string {
		return labelStyle.Render(label) + truncLeft(value, valWidth)
	}
	body := strings.Join([]string{
		labelStyle.Render("Project:  ") + name,
		line("Root:     ", s.projectRoot),
		line("Manifest: ", s.manifestRel),
	}, "\n")

	return headerStyle.Width(inner).Render(body)
}

// truncLeft keeps the right (most informative) end of a path, prefixing "…".
func truncLeft(s string, max int) string {
	if max < 4 {
		max = 4
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "…" + string(r[len(r)-(max-1):])
}

// ---------- breadcrumb / title bars ----------

// renderTitleBar renders text as a list-title-styled bar, so screens without
// their own list title keep a consistent header.
func renderTitleBar(text string) string {
	return listStyles.TitleBar.Render(listStyles.Title.Render(text))
}

// headerTitle is the shared header for a selected addon's screens, e.g.
// "MyAddon - Current:v1.0.0 - Versions". An empty section yields just the base.
func headerTitle(name, local, section string) string {
	cur := "none"
	if local != "" {
		cur = "v" + local
	}
	base := fmt.Sprintf("%s - Current:%s", name, cur)
	if section == "" {
		return base
	}
	return base + " - " + section
}

// pickSection describes the chosen asset for the confirm breadcrumb, e.g.
// "Assets v1.0.0 - addon.zip" or "Branches - main".
func pickSection(pick versionItem) string {
	if pick.branch {
		return "Branches - " + pick.tag
	}
	return fmt.Sprintf("Assets %s - %s", pick.tag, pick.asset.Name)
}

// ---------- confirm/summary box ----------

// confirmWidth is the inner width of the boxed confirm/input screens, sized to
// the terminal with a sane floor.
func (s *shared) confirmWidth() int {
	inner := s.width - 10
	if inner < 24 {
		inner = 24
	}
	return inner
}

// box renders body inside the shared bordered confirm/summary box.
func (s *shared) box(body string) string {
	return boxStyle.Width(s.confirmWidth()).Render(body)
}

// ---------- help bars ----------

// helpView renders a list's own help bar on its own, so it can be placed below
// the status and output panes.
func helpView(l list.Model) string {
	return l.Styles.HelpStyle.Render(l.Help.View(l))
}

// bindingHelp renders a set of key bindings as a static help bar aligned with
// the real list help bars (used by confirm / form / task screens).
func (s *shared) bindingHelp(bindings []key.Binding) string {
	return listStyles.HelpStyle.Render(s.help.ShortHelpView(bindings))
}

// noteHelp renders a plain (non-interactive) note in the help bar position.
func (s *shared) noteHelp(text string) string {
	return listStyles.HelpStyle.Render(s.help.Styles.ShortDesc.Render(text))
}

// ---------- output / log pane ----------

// appendLog records a line. The router's resize re-sets the viewport content and
// keeps it scrolled to the newest entry (unless the user is scrolling it).
func (s *shared) appendLog(line string) {
	s.logs = append(s.logs, line)
}

// clearLogs empties the output pane and the status line, and returns focus to
// the list. The caller (the router) re-lays-out afterward.
func (s *shared) clearLogs() {
	s.logs = nil
	s.statusMsg = ""
	s.focus = focusList
	s.output.SetContent("")
}

// outputInnerWidth is the text width inside the output box (full width minus the
// side borders and the 1-col padding on each side).
func (s *shared) outputInnerWidth() int {
	w := s.width - 2 - 2 - 2 // header margin parity, side borders, padding
	if w < 10 {
		w = 10
	}
	return w
}

// outputContentHeight is the viewport height for the log: a fixed ~25% of the
// terminal height, the same in every mode, so the log stretches to fill a stable
// region (and scrolls past it) instead of growing line by line.
func (s *shared) outputContentHeight() int {
	n := s.height / 4
	if n < 3 {
		n = 3
	}
	return n
}

// outputBoxHeight is the total rows the output pane occupies (content + the top
// and bottom border lines) when there are logs to show, else 0.
func (s *shared) outputBoxHeight() int {
	if len(s.logs) == 0 {
		return 0
	}
	return s.outputContentHeight() + 2
}

// logContent renders the log lines for the viewport.
func (s *shared) logContent() string {
	var b strings.Builder
	for i, l := range s.logs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(logStyle.Render(l))
	}
	return b.String()
}

// outputView draws the scrollable log inside a bordered box whose top edge is
// interrupted by an "Output" legend (and a scroll hint while focused).
func (s *shared) outputView() string {
	color := borderColor
	label := "Output"
	if s.focus == focusOutput {
		color = focusedColor
		label = "Output · ↑/↓ scroll · tab/esc back"
	}

	inner := s.outputInnerWidth()
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderForeground(color).
		Padding(0, 1).
		Width(inner + 2) // inner text + the 1-col padding on each side
	content := box.Render(s.output.View())

	// Hand-draw the top border so the legend can sit mid-line. The run between
	// the corners is the same width as the bottom border: inner + 2 (padding).
	legend := "─ " + label + " "
	fill := (inner + 2) - lipgloss.Width(legend)
	if fill < 0 {
		fill = 0
	}
	top := lipgloss.NewStyle().Foreground(color).
		Render("┌" + legend + strings.Repeat("─", fill) + "┐")

	return top + "\n" + content
}

// ---------- text helpers ----------

// hardWrap breaks s into chunks of at most width runes (URLs have no spaces to
// word-wrap on, so we break unconditionally).
func hardWrap(s string, width int) string {
	if width < 8 {
		width = 8
	}
	r := []rune(s)
	var b strings.Builder
	for len(r) > width {
		b.WriteString(string(r[:width]))
		b.WriteByte('\n')
		r = r[width:]
	}
	b.WriteString(string(r))
	return b.String()
}

// blanks returns an n-line block of empty lines (height n) for use as a flexible
// filler/spacer in JoinVertical stacks.
func blanks(n int) string {
	if n < 1 {
		return ""
	}
	return strings.Repeat("\n", n-1)
}

func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

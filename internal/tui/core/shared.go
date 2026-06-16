package core

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
type Shared struct {
	ManifestPath string
	ProjectRoot  string
	manifestRel  string
	projectName  string
	hasProject   bool

	width  int
	height int

	Spinner     spinner.Model
	output      viewport.Model
	help        help.Model // renders static (non-list) help bars
	Logs        []string
	focus       focusArea
	StatusMsg   string
	OutputShown bool // whether the output box is rendered (toggled with o)

	Events chan InstallEvent // the in-flight streaming task channel
}

func NewShared(manifestPath, projectRoot string) *Shared {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	rel, err := filepath.Rel(projectRoot, manifestPath)
	if err != nil {
		rel = manifestPath
	}
	name, exists := addon.ProjectName(projectRoot)

	h := help.New()
	h.Styles.ShortKey = h.Styles.ShortKey.Foreground(MutedColor)
	h.Styles.ShortDesc = h.Styles.ShortDesc.Foreground(MutedColor)
	h.Styles.ShortSeparator = h.Styles.ShortSeparator.Foreground(MutedColor)

	return &Shared{
		ManifestPath: manifestPath,
		ProjectRoot:  projectRoot,
		manifestRel:  rel,
		projectName:  name,
		hasProject:   exists,
		Spinner:      sp,
		output:       viewport.New(0, 0),
		help:         h,
	}
}

var (
	// mutedColor is the secondary/muted gray (borders, labels, help, list
	// descriptions); logColor is brighter, near-white, for the output log text.
	MutedColor   = lipgloss.Color("247")
	logColor     = lipgloss.Color("252")
	BorderColor  = lipgloss.Color("245")
	FocusedColor = lipgloss.Color("212")

	StatusStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(FocusedColor)
	logStyle    = lipgloss.NewStyle().Foreground(logColor)

	// tab strip: sits under the header, active tab highlighted, inactive muted,
	// closed off from the content below by a full-width rule. The switch keys are
	// shown in the help bar (ShortHelp), not here.
	tabStripStyle  = lipgloss.NewStyle().Padding(0, 1)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(FocusedColor)
	tabStyle       = lipgloss.NewStyle().Padding(0, 1).Foreground(MutedColor)
	tabRuleStyle   = lipgloss.NewStyle().Foreground(BorderColor)
	boxStyle       = lipgloss.NewStyle().Margin(1, 2).Padding(1, 2).Border(lipgloss.RoundedBorder())
	headerStyle    = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(BorderColor)
	labelStyle     = lipgloss.NewStyle().Foreground(MutedColor)

	// listStyles are the default bubbles list styles, reused to render
	// breadcrumb/title bars and static help so they align with the real lists.
	listStyles = list.DefaultStyles()
)

// focusArea tracks which pane receives navigation keys.
type focusArea int

const (
	focusList focusArea = iota
	focusOutput
)

// ---------- header ----------

// headerView renders the persistent context box shown on every screen.
func (s *Shared) headerView() string {
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
		line("Root:     ", s.ProjectRoot),
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
func RenderTitleBar(text string) string {
	return listStyles.TitleBar.Render(listStyles.Title.Render(text))
}

// headerTitle is the shared header for a selected addon's screens, e.g.
// "MyAddon - Current:v1.0.0 - Versions". An empty section yields just the base.
func HeaderTitle(name, local, section string) string {
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

// ---------- confirm/summary box ----------

// confirmWidth is the inner width of the boxed confirm/input screens, sized to
// the terminal with a sane floor.
func (s *Shared) ConfirmWidth() int {
	inner := s.width - 10
	if inner < 24 {
		inner = 24
	}
	return inner
}

// box renders body inside the shared bordered confirm/summary box.
func (s *Shared) Box(body string) string {
	return boxStyle.Width(s.ConfirmWidth()).Render(body)
}

// ---------- help bars ----------

// helpView renders a list's own help bar on its own, so it can be placed below
// the status and output panes.
func HelpView(l list.Model) string {
	return l.Styles.HelpStyle.Render(l.Help.View(l))
}

// newSelectList builds a list styled like the others (no status bar, help drawn
// separately, esc/enter hints) for the versions and submenu screens. It's sized
// to zero; the owning screen's SetSize gives it real dimensions.
func NewSelectList(items []list.Item, title string, extra ...key.Binding) list.Model {
	l := list.New(items, NewDelegate(), 0, 0)
	l.Title = title
	StyleList(&l)
	keys := func() []key.Binding {
		return append([]key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}, extra...)
	}
	l.AdditionalShortHelpKeys = keys
	l.AdditionalFullHelpKeys = keys
	return l
}

// newDelegate is the shared list delegate with brightened description text.
func NewDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(MutedColor)
	d.Styles.DimmedDesc = d.Styles.DimmedDesc.Foreground(MutedColor)
	return d
}

// styleList applies the shared list config: hide the built-in status bar and
// help (help is drawn manually at the bottom), and brighten the help colors.
func StyleList(l *list.Model) {
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	// Drive list scrolling from the central keymap so an added scheme (e.g. wasd)
	// reaches lists too; FullHint keeps the list's own full (?) help reading well.
	l.KeyMap.CursorUp = FullHint("up", Keys.Up)
	l.KeyMap.CursorDown = FullHint("down", Keys.Down)
	l.Help.Styles.ShortKey = l.Help.Styles.ShortKey.Foreground(MutedColor)
	l.Help.Styles.ShortDesc = l.Help.Styles.ShortDesc.Foreground(MutedColor)
	l.Help.Styles.ShortSeparator = l.Help.Styles.ShortSeparator.Foreground(MutedColor)
	l.Help.Styles.FullKey = l.Help.Styles.FullKey.Foreground(MutedColor)
	l.Help.Styles.FullDesc = l.Help.Styles.FullDesc.Foreground(MutedColor)
	l.Help.Styles.FullSeparator = l.Help.Styles.FullSeparator.Foreground(MutedColor)
}

// helpMode selects a tab root's help-bar preset. The zero value is the decluttered
// minimal bar (nav · select · quit · more); helpTabbed adds the [ ] tab-switch hint.
type HelpMode int

const (
	HelpMinimal HelpMode = iota
	HelpTabbed
)

// ShortHelp renders a tab root's decluttered short help for the given preset; the
// full (?) help still lists everything via the list's own FullHelp. Tab roots use
// this instead of helpView so secondary keys (filter, output, clear) stay out of
// the short bar.
func ShortHelp(l list.Model, mode HelpMode) string {
	if l.Help.ShowAll {
		return l.Styles.HelpStyle.Render(l.Help.FullHelpView(l.FullHelp()))
	}
	short := []key.Binding{
		Hint("up", Keys.Up),
		Hint("down", Keys.Down),
		Hint("select", Keys.Select),
	}
	switch mode {
	case HelpTabbed:
		short = append(short, tabHint())
	case HelpMinimal:
		short = append(short, Hint("back", Keys.Back))
	}
	short = append(short, l.KeyMap.ShowFullHelp)
	return l.Styles.HelpStyle.Render(l.Help.ShortHelpView(short))
}

// bindingHelp renders a set of key bindings as a static help bar aligned with
// the real list help bars (used by confirm / form / task screens).
func (s *Shared) BindingHelp(bindings []key.Binding) string {
	return listStyles.HelpStyle.Render(s.help.ShortHelpView(bindings))
}

// noteHelp renders a plain (non-interactive) note in the help bar position.
func (s *Shared) NoteHelp(text string) string {
	return listStyles.HelpStyle.Render(s.help.Styles.ShortDesc.Render(text))
}

// ---------- output / log pane ----------

// appendLog records a line. The router's resize re-sets the viewport content and
// keeps it scrolled to the newest entry (unless the user is scrolling it).
func (s *Shared) AppendLog(line string) {
	s.Logs = append(s.Logs, line)
	s.OutputShown = true // new output auto-reveals the box
}

// clearLogs empties the output pane and the status line, and returns focus to
// the list. The caller (the router) re-lays-out afterward.
func (s *Shared) clearLogs() {
	s.Logs = nil
	s.StatusMsg = ""
	s.focus = focusList
	s.OutputShown = false
	s.output.SetContent("")
}

// outputInnerWidth is the text width inside the output box (full width minus the
// side borders and the 1-col padding on each side).
func (s *Shared) outputInnerWidth() int {
	w := s.width - 2 - 2 - 2 // header margin parity, side borders, padding
	if w < 10 {
		w = 10
	}
	return w
}

// outputContentHeight is the viewport height for the log: a fixed ~25% of the
// terminal height, the same in every mode, so the log stretches to fill a stable
// region (and scrolls past it) instead of growing line by line.
func (s *Shared) outputContentHeight() int {
	n := s.height / 4
	if n < 3 {
		n = 3
	}
	return n
}

// outputBoxHeight is the total rows the output pane occupies (content + the top
// and bottom border lines) when shown, else 0.
func (s *Shared) OutputBoxHeight() int {
	if !s.OutputShown {
		return 0
	}
	return s.outputContentHeight() + 2
}

// logContent renders the log lines for the viewport.
func (s *Shared) logContent() string {
	var b strings.Builder
	for i, l := range s.Logs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(logStyle.Render(l))
	}
	return b.String()
}

// outputView draws the scrollable log inside a bordered box whose top edge is
// interrupted by an "Output" legend (and a scroll hint while focused).
func (s *Shared) OutputView() string {
	color := BorderColor
	label := "Output"
	if s.focus == focusOutput {
		color = FocusedColor
		label = "Output · ↑/↓ scroll · tab/esc back · o hide"
	}

	inner := s.outputInnerWidth()
	Box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderForeground(color).
		Padding(0, 1).
		Width(inner + 2) // inner text + the 1-col padding on each side
	content := Box.Render(s.output.View())

	// Hand-draw the top border so the legend can sit mid-line. The run between
	// the corners is the same width as the bottom border: inner + 2 (padding).
	legend := "─ " + label + " "
	fill := (inner + 2) - lipgloss.Width(legend)
	if fill < 0 {
		fill = 0
	}
	Top := lipgloss.NewStyle().Foreground(color).
		Render("┌" + legend + strings.Repeat("─", fill) + "┐")

	return Top + "\n" + content
}

// ---------- text helpers ----------

// hardWrap breaks s into chunks of at most width runes (URLs have no spaces to
// word-wrap on, so we break unconditionally).
func HardWrap(s string, width int) string {
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
func Blanks(n int) string {
	if n < 1 {
		return ""
	}
	return strings.Repeat("\n", n-1)
}

func IndentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

package core

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// Shared holds the cross-cutting state owned by the router: the consumer's own
// context (App), terminal size, the spinner, a help model for rendering static help
// bars, the in-flight task channel, and the optional Chrome (header/status/output —
// see chrome.go). A single instance is created in NewShared and pointed at by the
// router; screens receive it as a method argument. The framework names no domain
// type: App carries whatever struct the consumer wants (recover it typed with
// App[T]); the header renderer and output pane ride on Chrome.
type Shared struct {
	App    any     // consumer-owned context; recover it with App[T]
	Chrome *Chrome // optional header/status/output furniture (nil ⇒ fullscreen)

	width  int
	height int

	Spinner spinner.Model
	help    help.Model // renders static (non-list) help bars

	Events chan TaskEvent // the in-flight streaming task channel
}

func NewShared(app any) *Shared {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	h := help.New()
	h.Styles.ShortKey = h.Styles.ShortKey.Foreground(MutedColor)
	h.Styles.ShortDesc = h.Styles.ShortDesc.Foreground(MutedColor)
	h.Styles.ShortSeparator = h.Styles.ShortSeparator.Foreground(MutedColor)

	return &Shared{
		App:     app,
		Spinner: sp,
		help:    h,
	}
}

// Log appends a line to the output pane when one is present and supports logging
// (the default LogPane does), and is a no-op for a chromeless app or a non-logging
// Output — so context-agnostic callers (e.g. the task screen) needn't know the pane
// type or whether chrome exists.
func (s *Shared) Log(line string) {
	if s.Chrome == nil || s.Chrome.Output == nil {
		return
	}
	if l, ok := s.Chrome.Output.(interface{ Log(string) }); ok {
		l.Log(line)
	}
}

// SetStatus sets the transient status line shown under the body (no-op without
// chrome).
func (s *Shared) SetStatus(msg string) {
	if s.Chrome != nil {
		s.Chrome.Status = msg
	}
}

// App recovers the consumer's context from a Shared, type-asserted to *T. The
// consumer stores a *T in NewShared and reads it back here, so the framework stays
// domain-agnostic while tabs get a typed handle: c := core.App[MyCtx](sh).
func App[T any](s *Shared) *T { return s.App.(*T) }

// Width reports the current terminal width, so a Header closure can size/truncate
// its content to fit (see HeaderInnerWidth).
func (s *Shared) Width() int { return s.width }

// The palette and its derived styles are owned by the theme (see theme.go). The
// four colors are the secondary/muted gray (borders, labels, help, list
// descriptions), the brighter near-white log text, the border gray, and the
// selection accent. applyTheme reassigns the colors and rebuildStyles rebuilds
// everything below from them; init applies the default theme at startup.
var (
	MutedColor     lipgloss.Color
	logColor       lipgloss.Color
	BorderColor    lipgloss.Color
	FocusedColor   lipgloss.Color
	OnFocusedColor lipgloss.Color // text drawn on the accent (title bar)

	StatusStyle lipgloss.Style
	logStyle    lipgloss.Style

	// tab strip: sits under the header, active tab highlighted, inactive muted,
	// closed off from the content below by a full-width rule. The switch keys are
	// shown in the help bar (ShortHelp), not here.
	tabStripStyle  lipgloss.Style
	activeTabStyle lipgloss.Style
	tabStyle       lipgloss.Style
	tabRuleStyle   lipgloss.Style
	boxStyle       lipgloss.Style
	headerStyle    lipgloss.Style
	labelStyle     lipgloss.Style

	// listStyles are the bubbles list styles, reused to render breadcrumb/title
	// bars and static help so they align with the real lists. rebuildStyles resets
	// them from the defaults and themes the title bar each apply.
	listStyles list.Styles
)

func init() { applyTheme(current) }

// rebuildStyles rebuilds the derived styles from the current palette. applyTheme
// calls it after swapping colors so a theme switch repaints every chrome element.
func rebuildStyles() {
	StatusStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(FocusedColor)
	logStyle = lipgloss.NewStyle().Foreground(logColor)

	tabStripStyle = lipgloss.NewStyle().Padding(0, 1)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(FocusedColor)
	tabStyle = lipgloss.NewStyle().Padding(0, 1).Foreground(MutedColor)
	tabRuleStyle = lipgloss.NewStyle().Foreground(BorderColor)
	boxStyle = lipgloss.NewStyle().Margin(1, 2).Padding(1, 2).Border(lipgloss.RoundedBorder())
	headerStyle = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(BorderColor)
	labelStyle = lipgloss.NewStyle().Foreground(MutedColor)

	// Reset the list styles from the defaults, then theme the title bar so
	// breadcrumbs (RenderTitleBar) and list titles (StyleList) follow the accent
	// instead of bubbles' built-in purple. OnFocusedColor is the theme's text-on-
	// accent color, so a dark accent can still read.
	listStyles = list.DefaultStyles()
	listStyles.Title = listStyles.Title.Background(FocusedColor).Foreground(OnFocusedColor)
}

// ---------- header ----------

// HeaderInnerWidth is the content width inside the persistent context box for a
// terminal of the given width, so a Header closure can size/truncate values to fit.
func HeaderInnerWidth(width int) int {
	inner := width - 4 // minus border (2) and padding (2)
	if inner < 20 {
		inner = 20
	}
	return inner
}

// HeaderBox renders body inside the persistent bordered context box, sized to the
// terminal width. A consumer's Header closure builds body (e.g. with Label +
// TruncLeft) and returns HeaderBox(sh.Width(), body).
func HeaderBox(width int, body string) string {
	return headerStyle.Width(HeaderInnerWidth(width)).Render(body)
}

// Label renders a context-box/field label in the muted label style.
func Label(s string) string { return labelStyle.Render(s) }

// TruncLeft keeps the right (most informative) end of a path, prefixing "…".
func TruncLeft(s string, max int) string {
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

// newDelegate is the shared list delegate with brightened description text and the
// selected row recolored to the theme accent (bubbles' default selected styles are
// a hardcoded pink). The left-border layout from the default delegate is kept; only
// the colors change.
func NewDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(MutedColor)
	d.Styles.DimmedDesc = d.Styles.DimmedDesc.Foreground(MutedColor)
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(FocusedColor).BorderForeground(FocusedColor)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(FocusedColor).BorderForeground(FocusedColor)
	return d
}

// styleList applies the shared list config: hide the built-in status bar and
// help (help is drawn manually at the bottom), and brighten the help colors.
func StyleList(l *list.Model) {
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	// Theme the list's own title bar to match the breadcrumb (RenderTitleBar)
	// instead of bubbles' default purple.
	l.Styles.Title = listStyles.Title
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

// ---------- output / log styling ----------

// LogStyle is the themed style for output/log text, exported so a custom (or the
// default components) output pane renders log lines in the active palette. Read at
// render time, it picks up theme switches (rebuildStyles reassigns logStyle).
func LogStyle() lipgloss.Style { return logStyle }

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

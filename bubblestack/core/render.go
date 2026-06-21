package core

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

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

// Value renders a context-box field value in the log (near-white) style.
func Value(s string) string { return logStyle.Render(s) }

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
// their own list title keep a consistent header. The bubbles default TitleBar
// carries a bottom padding of 1; we drop it (keeping the left pad) so the
// breadcrumb sits flush against the body below it.
func RenderTitleBar(text string) string {
	return listStyles.TitleBar.Render(listStyles.Title.Render(text))
}

// WithTitle prepends a styled title bar to body, or returns body unchanged when
// title is empty — so any screen can make its in-body title optional by passing the
// raw (unrendered) title text straight through.
func WithTitle(title, body string) string {
	if title == "" {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, RenderTitleBar(title), body)
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
	if title != "" {
		l.Title = title
	} else {
		l.SetShowTitle(false)
	}
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
	// l.Styles.TitleBar = l.Styles.TitleBar.Margin(0) // how to set themes
	// Drive list scrolling from the central keymap so an added scheme (e.g. wasd)
	// reaches lists too; FullHint keeps the list's own full (?) help reading well.
	l.KeyMap.CursorUp = FullHint("up", Keys.Up)
	l.KeyMap.CursorDown = FullHint("down", Keys.Down)
	l.KeyMap.PrevPage = FullHint("prev page", Keys.Left)
	l.KeyMap.NextPage = FullHint("next page", Keys.Right)
	// Quitting is owned by the router's global q handler; drop the list's built-in
	// q/esc quit so esc at a tab root is a no-op (back is spam-safe to the root).
	l.KeyMap.Quit = key.NewBinding()
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

// styleHelp re-styles the static help model from the live MutedColor so static help
// bars track the active theme after a SetTheme switch. Built per call (not baked in
// at NewShared) for the same reason StyleList / fieldLabel restyle per call rather
// than caching a color that goes stale on the next theme change.
func (s *Shared) styleHelp() {
	s.help.Styles.ShortKey = s.help.Styles.ShortKey.Foreground(MutedColor)
	s.help.Styles.ShortDesc = s.help.Styles.ShortDesc.Foreground(MutedColor)
	s.help.Styles.ShortSeparator = s.help.Styles.ShortSeparator.Foreground(MutedColor)
}

// bindingHelp renders a set of key bindings as a static help bar aligned with
// the real list help bars (used by confirm / form / task screens).
func (s *Shared) BindingHelp(bindings []key.Binding) string {
	s.styleHelp()
	return listStyles.HelpStyle.Render(s.help.ShortHelpView(bindings))
}

// noteHelp renders a plain (non-interactive) note in the help bar position.
func (s *Shared) NoteHelp(text string) string {
	s.styleHelp()
	return listStyles.HelpStyle.Render(s.help.Styles.ShortDesc.Render(text))
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

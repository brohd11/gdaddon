package core

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

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

	statusStyle lipgloss.Style
	logStyle    lipgloss.Style

	// tab strip: sits under the header, active tab highlighted, inactive muted,
	// closed off from the content below by a full-width rule. The switch keys are
	// shown in the help bar (ShortHelp), not here.
	tabStripStyle  lipgloss.Style
	activeTabStyle lipgloss.Style
	tabStyle       lipgloss.Style
	tabRuleStyle   lipgloss.Style

	// breadcrumb bar: drawn by the router under the tab strip from the live nav
	// stack. Upstream segments + separators are muted; the current (top) segment
	// takes the accent. See RenderBreadcrumb.
	breadcrumbBarStyle  lipgloss.Style
	breadcrumbRuleStyle lipgloss.Style
	crumbMutedStyle     lipgloss.Style
	crumbCurStyle       lipgloss.Style

	boxStyle    lipgloss.Style
	headerStyle lipgloss.Style
	labelStyle  lipgloss.Style

	// listStyles are the bubbles list styles, reused to render breadcrumb/title
	// bars and static help so they align with the real lists. rebuildStyles resets
	// them from the defaults and themes the title bar each apply.
	listStyles list.Styles
)

func init() { applyTheme(current) }

// rebuildStyles rebuilds the derived styles from the current palette. applyTheme
// calls it after swapping colors so a theme switch repaints every chrome element.
func rebuildStyles() {
	statusStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(FocusedColor)
	logStyle = lipgloss.NewStyle().Foreground(logColor)

	tabStripStyle = lipgloss.NewStyle().Padding(0, 1)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(FocusedColor)
	tabStyle = lipgloss.NewStyle().Padding(0, 1).Foreground(MutedColor)
	tabRuleStyle = lipgloss.NewStyle().Foreground(BorderColor)

	breadcrumbBarStyle = lipgloss.NewStyle().Padding(0, 1)
	breadcrumbRuleStyle = lipgloss.NewStyle().Foreground(BorderColor)
	crumbMutedStyle = lipgloss.NewStyle().Foreground(MutedColor)
	crumbCurStyle = lipgloss.NewStyle().Foreground(FocusedColor).Bold(true)

	// No top margin: the gap above the box is owned by WithTitle so the body can
	// hug the top when no title bar is rendered. Margin order is top,right,bottom,left.
	boxStyle = lipgloss.NewStyle().Margin(0, 2, 1, 2).Padding(1, 2).Border(lipgloss.RoundedBorder())
	headerStyle = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(BorderColor)
	labelStyle = lipgloss.NewStyle().Foreground(MutedColor)

	// Reset the list styles from the defaults, then theme the title bar so
	// breadcrumbs (RenderTitleBar) and list titles (StyleList) follow the accent
	// instead of bubbles' built-in purple. OnFocusedColor is the theme's text-on-
	// accent color, so a dark accent can still read.
	listStyles = list.DefaultStyles()
	listStyles.Title = listStyles.Title.Background(FocusedColor).Foreground(OnFocusedColor)
}

// LogStyle is the themed style for output/log text, exported so a custom (or the
// default components) output pane renders log lines in the active palette. Read at
// render time, it picks up theme switches (rebuildStyles reassigns logStyle).
func LogStyle() lipgloss.Style { return logStyle }

// StatusStyle is the themed style for the transient status line, exported so a custom
// (or the default components) status element renders in the active palette. Read at
// render time, it picks up theme switches (rebuildStyles reassigns statusStyle).
func StatusStyle() lipgloss.Style { return statusStyle }

package tui

import (
	"gdaddon/internal/addon"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// browseScreen is the permanent root: the addon list with the pinned Actions
// row. It shows the status line and output pane below the list.
type browseScreen struct {
	list list.Model
}

var _ filterer = (*browseScreen)(nil)
var _ outputViewer = (*browseScreen)(nil)

func newBrowseScreen(statuses []addon.Status) *browseScreen {
	l := list.New(addonListItems(statuses), newDelegate(), 0, 0)
	l.Title = "Godot Addons"
	styleList(&l)
	// The browse short help is rendered custom (see HelpView) to stay
	// uncluttered; these extras only show in the full (?) help.
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "output")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear log")),
		}
	}
	return &browseScreen{list: l}
}

func (s *browseScreen) Init(*shared) tea.Cmd { return nil }

func (s *browseScreen) filtering() bool   { return s.list.FilterState() == list.Filtering }
func (s *browseScreen) wantsOutput() bool { return true }

func (s *browseScreen) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
	if s.filtering() {
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		return s, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q":
			return s, tea.Quit
		case "enter":
			switch sel := s.list.SelectedItem().(type) {
			case menuItem:
				return s, push(newActionsScreen())
			case item:
				if !sel.status.Installable() {
					return s, nil
				}
				sh.statusMsg = ""
				a := sel.status.Addon
				ld := newLoadingScreen(a, sel.status.LocalVersion, "fetching versions…", fetchReleases(a.URL))
				return s, push(ld)
			}
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *browseScreen) View(sh *shared) string {
	// Order bottom-up: list, then status, then output.
	body := s.list.View()
	if sh.statusMsg != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, statusStyle.Render(sh.statusMsg))
	}
	if len(sh.logs) > 0 {
		body = lipgloss.JoinVertical(lipgloss.Left, body, sh.outputView())
	}
	return body
}

// HelpView renders a decluttered short help (navigation + select + quit + more);
// filter, output, and clear-log live only in the full (?) help.
func (s *browseScreen) HelpView(*shared) string {
	l := s.list
	if l.Help.ShowAll {
		return l.Styles.HelpStyle.Render(l.Help.FullHelpView(l.FullHelp()))
	}
	short := []key.Binding{
		l.KeyMap.CursorUp,
		l.KeyMap.CursorDown,
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		l.KeyMap.Quit,
		l.KeyMap.ShowFullHelp,
	}
	return l.Styles.HelpStyle.Render(l.Help.ShortHelpView(short))
}

func (s *browseScreen) SetSize(sh *shared, width, bodyHeight int) {
	h := bodyHeight
	if sh.statusMsg != "" {
		h--
	}
	h -= sh.outputBoxHeight()
	if h < 1 {
		h = 1
	}
	s.list.SetSize(width, h)
}

// applyStatuses writes refreshed statuses back into the list, offset by 1 to skip
// the pinned Actions row (addon index i lives at list index i+1 — see
// addonListItems).
func (s *browseScreen) applyStatuses(statuses []addon.Status) {
	for i, st := range statuses {
		idx := i + 1
		if idx < len(s.list.Items()) {
			s.list.SetItem(idx, item{status: st})
		}
	}
}

// setItems rebuilds the list (handles a changed row count, unlike applyStatuses).
func (s *browseScreen) setItems(statuses []addon.Status) {
	s.list.SetItems(addonListItems(statuses))
}

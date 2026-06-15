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
var _ rootHandler = (*browseScreen)(nil)

func newBrowseScreen(statuses []addon.Status) *browseScreen {
	l := list.New(addonListItems(statuses), newDelegate(), 0, 0)
	l.Title = "Godot Addons"
	styleList(&l)
	// The browse short help is decluttered (see HelpView / rootHelp); these extras
	// only show in the full (?) help.
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
			sel, ok := s.list.SelectedItem().(item)
			if !ok || !sel.status.Installable() {
				return s, nil
			}
			sh.statusMsg = ""
			a := sel.status.Addon
			ld := newLoadingScreen(a, sel.status.LocalVersion, "fetching versions…", fetchReleases(a.URL))
			return s, push(ld)
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

// HelpView renders the decluttered tab-root help (nav · select · tabs · quit ·
// more); filter, output, and clear-log live only in the full (?) help.
func (s *browseScreen) HelpView(*shared) string { return rootHelp(s.list, helpTabbed) }

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

// handleRoot refreshes the browse list from a result message (rootHandler): the
// router unwinds to the root and hands the refresh here, keeping the browse-specific
// list logic out of the router.
func (s *browseScreen) handleRoot(sh *shared, msg tea.Msg) bool {
	m, ok := msg.(msgRootRefresh)
	if !ok {
		return false
	}
	sh.statusMsg = m.status
	if m.statuses != nil {
		if m.rebuild {
			s.setItems(m.statuses)
		} else {
			s.applyStatuses(m.statuses)
		}
	}
	return true
}

// applyStatuses writes refreshed statuses back into the list in place (row i ↔
// addon i; use setItems instead when the row count changed).
func (s *browseScreen) applyStatuses(statuses []addon.Status) {
	for i, st := range statuses {
		if i < len(s.list.Items()) {
			s.list.SetItem(i, item{status: st})
		}
	}
}

// setItems rebuilds the list (handles a changed row count, unlike applyStatuses).
func (s *browseScreen) setItems(statuses []addon.Status) {
	s.list.SetItems(addonListItems(statuses))
}

package tui

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/core"

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

var _ core.Filterer = (*browseScreen)(nil)
var _ core.OutputViewer = (*browseScreen)(nil)
var _ core.RootHandler = (*browseScreen)(nil)

func newBrowseScreen(statuses []addon.Status) *browseScreen {
	l := list.New(addonListItems(statuses), core.NewDelegate(), 0, 0)
	l.Title = "Godot Addons"
	core.StyleList(&l)
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

func (s *browseScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *browseScreen) Filtering() bool   { return s.list.FilterState() == list.Filtering }
func (s *browseScreen) WantsOutput() bool { return true }

func (s *browseScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	if s.Filtering() {
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
			sh.StatusMsg = ""
			a := sel.status.Addon
			return s, core.Push(newReleasesLoading(a, sel.status.LocalVersion))
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *browseScreen) View(sh *core.Shared) string {
	// Order bottom-up: list, then status, then output.
	body := s.list.View()
	if sh.StatusMsg != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, core.StatusStyle.Render(sh.StatusMsg))
	}
	if len(sh.Logs) > 0 {
		body = lipgloss.JoinVertical(lipgloss.Left, body, sh.OutputView())
	}
	return body
}

// HelpView renders the decluttered tab-root help (nav · select · tabs · quit ·
// more); filter, output, and clear-log live only in the full (?) help.
func (s *browseScreen) HelpView(*core.Shared) string { return core.RootHelp(s.list, core.HelpTabbed) }

func (s *browseScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	h := bodyHeight
	if sh.StatusMsg != "" {
		h--
	}
	h -= sh.OutputBoxHeight()
	if h < 1 {
		h = 1
	}
	s.list.SetSize(width, h)
}

// handleRoot refreshes the browse list from a result message (rootHandler): the
// router unwinds to the root and hands the refresh here, keeping the browse-specific
// list logic out of the router.
func (s *browseScreen) HandleRoot(sh *core.Shared, msg tea.Msg) bool {
	m, ok := msg.(core.MsgRootRefresh)
	if !ok {
		return false
	}
	sh.StatusMsg = m.Status
	if m.Statuses != nil {
		if m.Rebuild {
			s.setItems(m.Statuses)
		} else {
			s.applyStatuses(m.Statuses)
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

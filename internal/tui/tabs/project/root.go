package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// browseScreen is the permanent root: the addon list with the pinned Actions
// row. It shows the status line and output pane below the list.
type ProjectScreen struct {
	list list.Model
}

var _ core.Filterer = (*ProjectScreen)(nil)
var _ core.OutputViewer = (*ProjectScreen)(nil)
var _ core.RootHandler = (*ProjectScreen)(nil)

func NewProjectScreen(statuses []addon.Status) *ProjectScreen {
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
	return &ProjectScreen{list: l}
}

func (s *ProjectScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *ProjectScreen) Filtering() bool   { return s.list.FilterState() == list.Filtering }
func (s *ProjectScreen) WantsOutput() bool { return true }

func (s *ProjectScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
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
			if it, ok := s.list.SelectedItem().(components.Item); ok && it.Pick != nil {
				sh.StatusMsg = ""
				return s, it.Pick(sh)
			}
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *ProjectScreen) View(sh *core.Shared) string {
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
func (s *ProjectScreen) HelpView(*core.Shared) string { return core.RootHelp(s.list, core.HelpTabbed) }

func (s *ProjectScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
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

// HandleRoot rebuilds the browse list by re-inspecting the manifest (rootHandler):
// the router unwinds to the root and hands the refresh here, keeping the
// browse-specific list logic out of the router.
func (s *ProjectScreen) HandleRoot(sh *core.Shared, msg tea.Msg) bool {
	m, ok := msg.(core.MsgRefresh)
	if !ok || m.Target != core.RefreshProject {
		return false
	}
	sh.StatusMsg = m.Status
	if statuses, err := addon.Inspect(sh.ManifestPath, sh.ProjectRoot); err == nil {
		s.list.SetItems(addonListItems(statuses))
	}
	return true
}

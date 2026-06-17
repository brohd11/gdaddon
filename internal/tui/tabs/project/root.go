package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// browseScreen is the permanent root: the addon list with the pinned Actions
// row. It shows the status line and output pane below the list.
type ProjectScreen struct {
	list list.Model
}

var _ core.Filterer = (*ProjectScreen)(nil)
var _ core.RootHandler = (*ProjectScreen)(nil)

func NewProjectScreen(sh *core.Shared) *ProjectScreen {
	l := list.New(addonListItems(inspect(sh)), core.NewDelegate(), 0, 0)
	l.Title = "Godot Addons"
	core.StyleList(&l)
	// The browse short help is decluttered (see HelpView / ShortHelp); these extras
	// only show in the full (?) help.
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			core.FullHint("select", core.Keys.Select),
			core.FullHint("focus log", core.Keys.ToggleOutput),
			core.FullHint("toggle log", core.Keys.Output),
			core.FullHint("clear log", core.Keys.Clear),
		}
	}
	return &ProjectScreen{list: l}
}

func (s *ProjectScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *ProjectScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *ProjectScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	return s, components.RootUpdate(sh, &s.list, msg)
}

// View renders just the addon list; the status line and output box are drawn by
// the router as shared chrome below every screen.
func (s *ProjectScreen) View(*core.Shared) string { return s.list.View() }

// HelpView renders the decluttered tab-root help (nav · select · tabs · quit ·
// more); filter, output, and clear-log live only in the full (?) help.
func (s *ProjectScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpTabbed) }

func (s *ProjectScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

// HandleRoot rebuilds the browse list by re-inspecting the manifest (rootHandler):
// the router unwinds to the root and hands the refresh here, keeping the
// browse-specific list logic out of the router.
func (s *ProjectScreen) HandleRoot(sh *core.Shared, msg tea.Msg) bool {
	m, ok := msg.(core.MsgRefresh)
	if !ok || m.Target != appctx.Project {
		return false
	}
	sh.SetStatus(m.Status)
	s.list.SetItems(addonListItems(inspect(sh)))
	return true
}

// inspect reads the manifest's current state from the context paths, so the root
// builds (and refreshes) itself from disk rather than being handed statuses. A
// parse/read error yields no rows (an empty list), matching the global/archive tabs.
func inspect(sh *core.Shared) []addon.Status {
	c := appctx.Of(sh)
	statuses, _ := addon.Inspect(c.ManifestPath, c.ProjectRoot)
	return statuses
}

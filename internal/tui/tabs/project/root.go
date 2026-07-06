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

// projectTitle is the browse list's base Title; the active sort mode is appended.
const projectTitle = "Godot Addons"

// browseScreen is the permanent root: the addon list with the pinned Actions
// row. It shows the status line and output pane below the list.
type ProjectScreen struct {
	list list.Model
	sort appctx.SortMode
}

var _ core.Filterer = (*ProjectScreen)(nil)
var _ core.Receiver = (*ProjectScreen)(nil)
var _ core.Crumber = (*ProjectScreen)(nil)

// CrumbLabel anchors the breadcrumb at the Project root.
func (s *ProjectScreen) CrumbLabel(bool) string { return "Tab" } // s.list.Title }

func NewProjectScreen(sh *core.Shared) *ProjectScreen {
	l := list.New(projectListItems(sh, appctx.SortAlpha), core.NewDelegate(), 0, 0)
	l.Title = appctx.SortTitle(projectTitle, appctx.SortAlpha)
	core.StyleList(&l)
	// The browse short help is decluttered (see HelpView / ShortHelp); these extras
	// only show in the full (?) help.
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			core.FullHint("select", core.Keys.Select),
			core.FullHint("sort", appctx.AppKeys.Sort),
			core.FullHint("terminal (git)", appctx.AppKeys.Terminal),
			core.FullHint("focus log", core.Keys.ToggleOutput),
			core.FullHint("toggle log", core.Keys.Output),
			core.FullHint("clear log", core.Keys.Clear),
		}
	}
	return &ProjectScreen{list: l}
}

// Init kicks off the initial update check so the "update available" markers fill
// in asynchronously once the release listings come back.
func (s *ProjectScreen) Init(sh *core.Shared) tea.Cmd { return checkUpdatesCmd(sh) }

func (s *ProjectScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *ProjectScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	// "i" cycles the sort order (A→Z / Z→A / status), rebuilding the list in place.
	// Gated behind the filter guard so it doesn't hijack filter typing.
	if k, ok := msg.(tea.KeyMsg); ok && !s.Filtering() && core.MatchKey(k.String(), appctx.AppKeys.Sort) {
		appctx.CycleSort(&s.list, &s.sort, projectSortModes, projectTitle,
			func(m appctx.SortMode) []list.Item { return projectListItems(sh, m) })
		return s, core.Action{}
	}
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

// Receive rebuilds the browse list by re-inspecting the manifest on a ProjectDirty
// (manifest contents changed) or PathRefresh (the manifest path itself changed, e.g.
// just created) broadcast, keeping the browse-specific list logic out of the router.
// The status line and any focus switch are composed at the call site (core.Seq).
func (s *ProjectScreen) Receive(sh *core.Shared, payload any) core.Action {
	switch p := payload.(type) {
	case appctx.ProjectDirty, appctx.PathRefresh:
		appctx.Of(sh).RefreshProject()
		s.list.SetItems(projectListItems(sh, s.sort))
		// Re-run the update check against the refreshed manifest; the markers
		// fill back in when its results broadcast.
		return core.Async(checkUpdatesCmd(sh))
	case updateChecksReady:
		appctx.Of(sh).SetUpdateChecks(p.checks)
		s.list.SetItems(projectListItems(sh, s.sort))
	}
	return core.Action{}
}

// inspect reads the manifest's current state from the context paths, so the root
// builds (and refreshes) itself from disk rather than being handed statuses. A
// parse/read error yields no rows (an empty list), matching the global/archive tabs.
func inspect(sh *core.Shared) []addon.Status {
	c := appctx.Of(sh)
	statuses, _ := addon.Inspect(c.ManifestPath, c.ProjectRoot)
	return statuses
}

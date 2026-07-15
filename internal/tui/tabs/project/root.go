package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	gitflow "gdaddon/internal/tui/flows/git"

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
	// fetching marks a fetch-all pass in flight (see fetchAllCmd), so a second "f"
	// while the first is still running doesn't fan out a duplicate set of fetches.
	// Cleared when its fetchDone arrives.
	fetching bool
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
			core.FullHint("terminal", appctx.AppKeys.Terminal),
			core.FullHint("fetch", appctx.AppKeys.Fetch),
			core.FullHint("git", appctx.AppKeys.Git),
			core.FullHint("git all", appctx.AppKeys.GitAll),
			core.FullHint("focus log", core.Keys.ToggleOutput),
			core.FullHint("toggle log", core.Keys.Output),
			core.FullHint("wrap log", core.Keys.Wrap),
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
	// The tab's own keys, gated behind the filter guard so they don't hijack filter typing.
	if k, ok := msg.(tea.KeyMsg); ok && !s.Filtering() {
		switch {
		// "i" cycles the sort order (A→Z / Z→A / status), rebuilding the list in place.
		case core.MatchKey(k.String(), appctx.AppKeys.Sort):
			appctx.CycleSort(&s.list, &s.sort, projectSortModes, projectTitle,
				func(m appctx.SortMode) []list.Item { return projectListItems(sh, m) })
			return s, core.Action{}
		// "f" git-fetches every project checkout so the ahead/behind markers can see new
		// upstream commits. Network-bound, hence explicit — it never runs on its own.
		case core.MatchKey(k.String(), appctx.AppKeys.Fetch):
			if s.fetching {
				return s, core.SetStatus("fetch already running")
			}
			s.fetching = true
			return s, core.Seq(
				core.SetStatus("fetching git checkouts…"),
				core.Async(fetchAllCmd(sh)),
			)
		// "V" opens the project-wide Git page (fetch/pull/push across every checkout). "v" is
		// per-row (an addon's own Git page) and lives in the row's Item.Keys instead.
		case core.MatchKey(k.String(), appctx.AppKeys.GitAll):
			return s, core.Push(gitflow.AllRepos(sh))
		}
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
	case appctx.GitRefresh:
		// A git operation (pull/push/commit/single-repo fetch) changed a checkout: recompute
		// the local git state so the dirty / ahead / behind markers settle. Local-only, so
		// unlike ProjectDirty it doesn't re-fire the network update check.
		appctx.Of(sh).RefreshProject()
		s.list.SetItems(projectListItems(sh, s.sort))
	case fetchDone:
		// The refs are now current, so re-inspecting recomputes each checkout's ahead/behind
		// (RefreshProject → refreshGitChecks) and the markers appear. Logging here — rather
		// than from the cmd — keeps Shared on the UI thread; a plain ProjectDirty would also
		// work but would re-fire the network-bound update check for no reason.
		s.fetching = false
		for _, r := range p.results {
			sh.Log(fetchLine(r))
		}
		appctx.Of(sh).RefreshProject()
		s.list.SetItems(projectListItems(sh, s.sort))
		if len(p.results) == 0 {
			return core.SetStatus("no git checkouts to fetch")
		}
		line, failed := fetchSummary(p.results)
		return core.SetStatusAndLog(line, failed) // force the log open only to show a failure's reason
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

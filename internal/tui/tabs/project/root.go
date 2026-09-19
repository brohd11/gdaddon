package project

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	gitflow "github.com/brohd11/gdaddon/internal/tui/flows/git"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repoui"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
)

// projectTitle is the browse list's base Title; the active sort mode is appended.
const projectTitle = "Project"

// projectState owns the Project tab's domain state; RootListScreen handles its UI.
type projectState struct {
	screen *components.RootListScreen
	sort   appctx.SortMode
	// fetching prevents duplicate fetch-all work until FetchDoneMsg arrives.
	fetching bool
}

func NewProjectScreen(sh *core.Shared) *components.RootListScreen {
	s := &projectState{}
	opts := appctx.RootListOpts(sh, appctx.SortTitle(projectTitle, s.sort))
	opts.Help = []key.Binding{
		core.FullHint("sort", appctx.AppKeys.Sort),
		core.FullHint("terminal", appctx.AppKeys.Terminal),
		core.FullHint("term window", appctx.AppKeys.TerminalWindow),
		core.FullHint("open dir", appctx.AppKeys.OpenDir),
		core.FullHint("fetch", appctx.AppKeys.Fetch),
		core.FullHint("git", appctx.AppKeys.Git),
		core.FullHint("diff", appctx.AppKeys.Diff),
		core.FullHint("git all", appctx.AppKeys.GitAll),
		core.FullHint("root git", appctx.AppKeys.RootGit),
		core.FullHint("focus log", core.Keys.ToggleOutput),
		core.FullHint("toggle log", core.Keys.Output),
		core.FullHint("wrap log", core.Keys.Wrap),
		core.FullHint("clear log", core.Keys.Clear),
	}
	// Update markers arrive asynchronously; refresh broadcasts can start another check.
	opts.Init = checkUpdatesCmd
	opts.OnKey = s.onKey
	opts.Receive = s.receive
	s.screen = components.NewRootList(projectListItems(sh, s.sort), opts)
	return s.screen
}

// onKey adds project-wide commands. The component guards filter typing and falls
// through to each row's own shortcuts when this callback does not handle a key.
func (s *projectState) onKey(sh *core.Shared, k string, _ list.Item) (core.Action, bool) {
	switch {
	case core.MatchKey(k, appctx.AppKeys.Sort):
		appctx.CycleSort(s.screen.List(), &s.sort, projectSortModes, projectTitle,
			func(m appctx.SortMode) []list.Item { return projectListItems(sh, m) })
		return core.Action{}, true
	case core.MatchKey(k, appctx.AppKeys.Fetch):
		if s.fetching {
			return core.SetStatus("fetch already running"), true
		}
		s.fetching = true
		return core.Seq(
			core.SetStatus("fetching git checkouts…"),
			core.Async(fetchAllCmd(sh)),
		), true
	case core.MatchKey(k, appctx.AppKeys.GitAll):
		return core.Push(gitflow.AllRepos(sh)), true
	case core.MatchKey(k, appctx.AppKeys.RootGit):
		return RootGitAction(sh), true
	}
	return core.Action{}, false
}

// RootGitAction opens the project repo's own Git page — the ctrl+v key and a header
// click both resolve to it. A non-checkout root only reports on the status line.
func RootGitAction(sh *core.Shared) core.Action {
	root := appctx.Of(sh).RootRepo
	if root == nil {
		return core.SetStatus("project root is not a git checkout")
	}
	return core.Push(repoui.RepoMenu(sh, *root, root.Name))
}

// receive rebuilds the browse list by re-inspecting the manifest on a ProjectDirty
// (manifest contents changed) or PathRefresh (the manifest path itself changed, e.g.
// just created) broadcast, keeping the browse-specific list logic out of the router.
// The status line and any focus switch are composed at the call site (core.Seq).
func (s *projectState) receive(sh *core.Shared, payload any) core.Action {
	switch p := payload.(type) {
	case appctx.ProjectDirty, appctx.PathRefresh:
		s.reload(sh)
		// Re-run the update check against the refreshed manifest; the markers
		// fill back in when its results broadcast.
		return core.Async(checkUpdatesCmd(sh))
	case updateChecksReady:
		appctx.Of(sh).SetUpdateChecks(p.checks)
		s.screen.SetItems(projectListItems(sh, s.sort))
	case appctx.GitRefresh:
		// A batch git operation changed checkouts: recompute the local git state
		// so the dirty / ahead / behind markers settle. Local-only, so
		// unlike ProjectDirty it doesn't re-fire the network update check.
		s.reload(sh)
	case appctx.GitRepoRefresh:
		appctx.Of(sh).RefreshRepo(p)
		s.screen.SetItems(projectListItems(sh, s.sort))
	case repoui.FetchDoneMsg:
		// The refs are now current, so re-inspecting recomputes each checkout's ahead/behind
		// (RefreshProject → refreshGitChecks) and the markers appear. RefreshRoots then
		// rebuilds every tab root from the refreshed state; repoui.LogFetchResults writes the
		// per-repo lines and returns the summary (log forced open only on a failure).
		s.fetching = false
		s.reload(sh)
		return core.Seq(
			core.RefreshRoots(),
			repoui.LogFetchResults(sh, p.Results, "git checkout(s)", "no git checkouts to fetch"),
		)
	}
	return core.Action{}
}

// reload re-inspects the manifest and redraws the rows from it. Three of receive's
// branches need exactly this pair and the order is load-bearing — the rows are built from
// the context RefreshProject just repopulated — so it is one call, not two lines each time.
func (s *projectState) reload(sh *core.Shared) {
	appctx.Of(sh).RefreshProject()
	s.screen.SetItems(projectListItems(sh, s.sort))
}

// inspect uses the project's cached inspection so redraws do not query every repo.
func inspect(sh *core.Shared) []addon.Status {
	return appctx.Of(sh).ProjectStatuses()
}

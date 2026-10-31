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
	order  []string
	rows   map[string]list.Item
	shown  map[string]bool
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
	s.screen = components.NewRootList(s.rebuildItems(sh), opts)
	return s.screen
}

// onKey adds project-wide commands. The component guards filter typing and falls
// through to each row's own shortcuts when this callback does not handle a key.
func (s *projectState) onKey(sh *core.Shared, k string, _ list.Item) (core.Action, bool) {
	switch {
	case core.MatchKey(k, appctx.AppKeys.Sort):
		appctx.CycleSort(s.screen.List(), &s.sort, projectSortModes, projectTitle,
			func(appctx.SortMode) []list.Item { return s.rebuildItems(sh) })
		return core.Action{}, true
	case core.MatchKey(k, appctx.AppKeys.Fetch):
		cmd, started := fetchAllCmd(sh)
		if !started {
			return core.SetStatus("fetch already running"), true
		}
		return core.Seq(
			core.SetStatus("fetching git checkouts…"),
			core.Async(cmd),
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

// receive reloads the list asynchronously on ProjectDirty (contents changed), PathRefresh
// (path changed), GitRefresh and fetch completion, landing each entry's result as it
// arrives. Status and focus are composed by the caller.
func (s *projectState) receive(sh *core.Shared, payload any) core.Action {
	c := appctx.Of(sh)
	if ready, handled := c.ListUpdates.Receive(payload); handled {
		if ready {
			s.publish(sh)
		}
		return core.Action{}
	}
	if act, ok := c.Refresh.Receive(payload); ok {
		return act
	}
	if handled, changed, act := c.ApplyRefreshResult(payload); handled {
		if changed {
			if !c.Refresh.Running() {
				s.rebuild(sh)
			} else {
				return core.Seq(act, c.ListUpdates.Request())
			}
		}
		return act
	}
	switch p := payload.(type) {
	case appctx.ProjectDirty, appctx.PathRefresh:
		// Local state always refreshes; release checks share the editor's daily limit.
		return core.Seq(s.reload(sh), core.Async(checkUpdatesCmd(sh)))
	case updateChecksReady:
		if p.checks != nil {
			c.SetUpdateChecks(p.checks)
			s.rebuild(sh)
		}
		if p.stampErr != nil {
			return core.SetStatusAndLog(p.stampErr.Error())
		}
	case appctx.GitRefresh:
		// A batch git operation finished: recompute the local git state (no network update check).
		return s.reload(sh)
	case appctx.GitRepoRefresh:
		if c.RefreshRepo(p) {
			return s.reload(sh)
		}
		s.rebuild(sh)
	case repoui.FetchDoneMsg:
		if !c.Fetch.Complete(p) {
			return core.Action{}
		}
		// The refs are current: re-inspect so ahead/behind markers appear, rebuild the roots, and
		// log the per-repo results.
		return core.Seq(
			s.reload(sh),
			core.RefreshRoots(),
			repoui.LogFetchResults(sh, p.Results, "git checkout(s)", "no git checkouts to fetch"),
		)
	}
	return core.Action{}
}

// reload starts the async re-inspection and shows the rows pending right away. The status
// sort's order is frozen at its pre-refresh ranks until every result lands.
func (s *projectState) reload(sh *core.Shared) core.Action {
	c := appctx.Of(sh)
	ranks := statusRanks(sh)
	act := c.StartProjectRefresh()
	c.RankSnap = ranks
	// New entries have no prior rank; freeze their initial position until completion too.
	for _, status := range c.ProjectStatuses() {
		if _, ok := ranks[status.Addon.Name]; !ok {
			ranks[status.Addon.Name] = attentionRank(rowFor(c, status))
		}
	}
	s.rebuild(sh)
	return act
}

func (s *projectState) rebuildItems(sh *core.Shared) []list.Item {
	c := appctx.Of(sh)
	c.ListUpdates.Reset()
	clear(c.DirtyProjectRows)
	rows := projectRows(sh, s.sort)
	s.order = make([]string, 0, len(rows))
	s.rows = make(map[string]list.Item, len(rows))
	s.shown = make(map[string]bool, len(rows))
	for _, row := range rows {
		name := row.s.Addon.Name
		s.order = append(s.order, name)
		s.rows[name] = addonItem(row)
	}
	for _, row := range visibleRows(rows, s.sort) {
		s.shown[row.s.Addon.Name] = true
	}
	return s.projectItems()
}

func (s *projectState) rebuild(sh *core.Shared) { s.screen.SetItems(s.rebuildItems(sh)) }

func (s *projectState) projectItems() []list.Item {
	items := make([]list.Item, 0, len(s.order))
	for _, key := range s.order {
		if s.shown[key] {
			items = append(items, s.rows[key])
		}
	}
	return items
}

func (s *projectState) publish(sh *core.Shared) {
	c := appctx.Of(sh)
	for name := range c.DirtyProjectRows {
		if status, ok := c.ProjectStatus(name); ok {
			row := rowFor(c, status)
			s.rows[name] = addonItem(row)
			s.shown[name] = s.sort != appctx.SortStatusInstalled || row.s.Present() || row.unseen
		}
	}
	c.ListUpdates.Reset()
	clear(c.DirtyProjectRows)
	s.screen.SetItems(s.projectItems())
}

// inspect uses the project's cached inspection so redraws do not query every repo.
func inspect(sh *core.Shared) []addon.Status {
	return appctx.Of(sh).ProjectStatuses()
}

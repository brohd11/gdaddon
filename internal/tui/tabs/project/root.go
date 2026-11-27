package project

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	gitflow "github.com/brohd11/gdaddon/internal/tui/flows/git"
	"github.com/brohd11/gdaddon/internal/tui/widgets"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repoui"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
)

// projectTitle is the browse list's base Title; the active sort mode is appended.
const projectTitle = "Project"

// projectState owns the Project tab's domain state; a RootListScreen or, in the optional
// category view, a RootTreeScreen handles its UI. Exactly one of screen and tree is set.
type projectState struct {
	screen *components.RootListScreen
	tree   *components.RootTreeScreen
	sort   appctx.SortMode
	order  []string
	rows   map[string]list.Item
	shown  map[string]bool
}

// NewProjectRoot builds the Project tab in the session's view: the list, or the category
// tree when Ctx.ProjectTree is set (the View key flips it and rebuilds this root).
func NewProjectRoot(sh *core.Shared) core.Screen {
	if appctx.Of(sh).ProjectTree {
		return NewProjectTreeScreen(sh)
	}
	return NewProjectScreen(sh)
}

// NewProjectScreen builds the Project tab's list view.
func NewProjectScreen(sh *core.Shared) *components.RootListScreen {
	s := &projectState{sort: appctx.Of(sh).ProjectSort}
	opts := appctx.RootListOpts(sh, appctx.SortTitle(projectTitle, s.sort))
	opts.Help = projectHelp()
	// Update markers arrive asynchronously; refresh broadcasts can start another check.
	opts.Init = checkUpdatesCmd
	opts.OnKey = s.onKey
	opts.Receive = s.receive
	s.screen = components.NewRootList(s.rebuildItems(sh), opts)
	return s.screen
}

// NewProjectTreeScreen builds the Project tab's category view: the same rows, filed into
// folders by each addon's global-list category (Ctx.GlobalCategory).
func NewProjectTreeScreen(sh *core.Shared) *components.RootTreeScreen {
	c := appctx.Of(sh)
	s := &projectState{sort: c.ProjectSort}
	s.tree = components.NewRootTree(nil, components.RootTreeOpts{
		Title: appctx.SortTitle(projectTitle, s.sort),
		Help:  projectHelp(),
		Folds: c.ProjectTreeFolds(),
		Init:  checkUpdatesCmd,
		OnKey: func(sh *core.Shared, k string, _ components.TreeNode) (core.Action, bool) {
			return s.onKey(sh, k, nil)
		},
		Receive: s.receive,
	})
	s.rebuild(sh)
	return s.tree
}

// projectHelp is the (?) menu shared by both views.
func projectHelp() []key.Binding {
	return []key.Binding{
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
		core.FullHint("tree view", appctx.AppKeys.View),
	}
}

// onKey adds project-wide commands. The component guards filter typing and falls
// through to each row's own shortcuts when this callback does not handle a key.
func (s *projectState) onKey(sh *core.Shared, k string, _ list.Item) (core.Action, bool) {
	switch {
	case core.MatchKey(k, appctx.AppKeys.Sort):
		if s.tree != nil {
			s.sort = components.NextSort(s.sort, projectSortModes)
			s.rebuild(sh)
			s.tree.SetTitle(appctx.SortTitle(projectTitle, s.sort))
		} else {
			appctx.CycleSort(s.screen.List(), &s.sort, projectSortModes, projectTitle,
				func(appctx.SortMode) []list.Item { return s.rebuildItems(sh) })
		}
		appctx.Of(sh).ProjectSort = s.sort
		return core.Action{}, true
	case core.MatchKey(k, appctx.AppKeys.View):
		c := appctx.Of(sh)
		c.ProjectTree = !c.ProjectTree
		status := "list view"
		if c.ProjectTree {
			status = "tree view"
		}
		return core.Seq(core.SetStatus(status), core.RebuildRoot()), true
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
		// Local state always refreshes; release checks share the editor's daily limit, except
		// for entries whose pin just changed. reload must run first: it queues those.
		reload := s.reload(sh)
		return core.Seq(reload, core.Async(checkUpdatesCmd(sh)), core.Async(recheckChangedCmd(sh)))
	case updateChecksReady:
		if p.gen != c.UpdateGen.Load() {
			return core.Action{}
		}
		if p.checks != nil {
			c.SetUpdateChecks(p.checks)
			s.rebuild(sh)
		}
		if p.stampErr != nil {
			return core.SetStatusAndLog(p.stampErr.Error())
		}
	case updateRecheckReady:
		if p.root == c.ProjectRoot && c.MergeUpdateChecks(p.checks, p.keys) {
			s.rebuild(sh)
		}
	case appctx.GlobalDirty:
		// Categories come from the global list. The Global tab reloads it on this same
		// broadcast, but this tab may hear it first.
		if s.tree != nil {
			c.RefreshGlobal()
			s.show(sh)
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

func (s *projectState) rebuild(sh *core.Shared) {
	s.rebuildItems(sh)
	s.show(sh)
}

// show paints the current rows into whichever view this state drives.
func (s *projectState) show(sh *core.Shared) {
	if s.tree != nil {
		s.tree.SetNodes(s.treeNodes(sh))
		return
	}
	s.screen.SetItems(s.projectItems())
}

// treeNodes files the shown rows by their global category. Rows keep the sort order inside
// each folder; folders are by name, reversed only under Z→A.
func (s *projectState) treeNodes(sh *core.Shared) []components.TreeNode {
	c := appctx.Of(sh)
	leaves := make([]widgets.CategoryLeaf, 0, len(s.order))
	for _, name := range s.order {
		item, ok := s.rows[name].(components.Item)
		if !s.shown[name] || !ok {
			continue
		}
		category := ""
		if status, ok := c.ProjectStatus(name); ok {
			category = c.GlobalCategory(status.Addon.URL)
		}
		leaves = append(leaves, widgets.CategoryLeaf{
			ID:       projectLeafPrefix + name,
			Category: category,
			Item: components.CompactItem{
				Name: item.Name, Suffix: item.Desc, Pending: item.Pending, Pick: item.Pick, Keys: item.Keys,
			},
		})
	}
	return widgets.CategoryTree(leaves, s.sort == appctx.SortReverse)
}

// projectLeafPrefix starts an addon leaf's node ID; folds follow IDs.
const projectLeafPrefix = "addon:"

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
	s.show(sh)
}

// inspect uses the project's cached inspection so redraws do not query every repo.
func inspect(sh *core.Shared) []addon.Status {
	return appctx.Of(sh).ProjectStatuses()
}

package appctx

import (
	tea "charm.land/bubbletea/v2"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gitstack/repoui"
)

// RefreshAll broadcasts every Dirty marker so each tab root reloads (the Refresh key and
// Actions ▸ Refresh). The Project reload is async and reports "Refreshed" itself when it
// lands; ProjectDirty and PathRefresh both start it, the second superseding the first
// before any inspection runs.
func RefreshAll() core.Action {
	return core.Seq(
		core.PropagateAll(ArchiveDirty{}),
		core.PropagateAll(ProjectDirty{}),
		core.PropagateAll(GlobalDirty{}),
		core.PropagateAll(PathRefresh{}),
	)
}

// Tab titles, shared by the tab wiring and ShowTab callers.
const (
	TitleProject = "Project"
	TitleGlobal  = "Global"
	TitleSets    = "Sets"
	TitleArchive = "Archive"
	TitleActions = "Actions"
	TitleSearch  = "Search"
)

// Dirty payloads are "reload yourself" broadcasts: each tab root reloads on its own. Status
// and focus changes are composed at the call site with core.Seq.
type (
	ProjectDirty struct{}
	GlobalDirty  struct{}
	ArchiveDirty struct{}
	// SetsDirty is broadcast after a set is created or deleted, so the pushed Sets
	// submenu (Actions ▸ Sets) reloads its list from ~/.gdaddon/sets.
	SetsDirty struct{}
	// PathRefresh is broadcast after the manifest or project paths change, reloading the
	// Project list and Actions menu (the header reads the context each render).
	PathRefresh struct{}
)

// GitRefresh is the full local reload after batch git operations (no update check);
// single-checkout tasks use GitRepoRefresh.
type GitRefresh = repoui.RefreshMsg

// GitRepoRefresh identifies the single checkout changed by a git task.
type GitRepoRefresh = repoui.RepoRefreshMsg

type pathsFound struct {
	path string
	err  error
}

// RefreshPaths discovers paths off the UI thread when async, then applies them
// through Ctx.Receive before broadcasting PathRefresh. Sync scans inline.
func RefreshPaths(sh *core.Shared, async bool) tea.Cmd {
	if async {
		c := Of(sh)
		root := c.SearchRoot
		if root == "" {
			root = c.ProjectRoot
		}
		return func() tea.Msg {
			path, err := addon.FindManifest(root)
			return core.PropagateAll(pathsFound{path: path, err: err})
		}
	}
	Of(sh).Scan()
	return nil
}

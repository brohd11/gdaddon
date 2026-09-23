package appctx

import (
	tea "charm.land/bubbletea/v2"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repoui"
)

// RefreshAll broadcasts every Dirty marker so each tab root reloads (the Refresh key and
// Actions ▸ Refresh).
func RefreshAll() core.Action {
	return core.Seq(
		core.PropagateAll(ArchiveDirty{}),
		core.PropagateAll(ProjectDirty{}),
		core.PropagateAll(GlobalDirty{}),
		core.PropagateAll(PathRefresh{}),
		core.SetStatus("Refreshed"),
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

// RefreshPaths re-runs Scan after the paths may have changed. Async, it scans in a cmd
// and then broadcasts PathRefresh, so receivers see the new paths; sync, it scans inline
// without a broadcast.
func RefreshPaths(sh *core.Shared, async bool) tea.Cmd {
	if async {
		return func() tea.Msg {
			Of(sh).Scan()
			return core.PropagateAll(PathRefresh{})
		}
	}
	Of(sh).Scan()
	return nil
}

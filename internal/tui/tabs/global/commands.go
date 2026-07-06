package global

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/core"
)

// commitRemove removes the plugin from the global list, plus its archived packages
// when the chosen mode is "global + archive". On success it broadcasts GlobalDirty so
// the removed row disappears (and focuses Global); when it also deleted archive files
// it broadcasts ArchiveDirty so the Archive tab reloads too — silently, since focus
// stays on Global.
func commitRemove(sh *core.Shared, g globalItem, mode int) core.Action {
	archiveRemoved := false
	if mode == removeGlobalArchive {
		if repoID, err := source.RepoID(g.url); err == nil {
			if err := archive.RemoveRepo(repoID); err != nil {
				return core.SeqErr(err, core.ResetToRoot())
			}
			archiveRemoved = true
		}
		// A non-github url has no archive to remove; fall through to the entry removal.
	}

	globalPath, err := addon.GlobalListPath()
	if err == nil {
		err = addon.RemoveEntry(globalPath, g.name)
	}
	if err != nil {
		return core.SeqErr(err, core.ResetToRoot())
	}
	global := core.Seq(
		core.SetStatus("removed "+g.name),
		core.PropagateAll(appctx.GlobalDirty{}),
		core.ShowTab(appctx.TitleGlobal),
	)
	if archiveRemoved {
		return core.Seq(global, core.PropagateAll(appctx.ArchiveDirty{}))
	}
	return global
}

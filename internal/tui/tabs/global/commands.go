package global

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// commitRemove removes the plugin from the global list, plus its archived packages
// when the chosen mode is "global + archive". On success it refreshes the Global
// tab so the removed row disappears.
func commitRemove(sh *core.Shared, g globalItem, mode int) tea.Cmd {
	if mode == removeGlobalArchive {
		if repoID, err := source.RepoID(g.url); err == nil {
			if err := archive.RemoveRepo(repoID); err != nil {
				sh.SetStatus("error: " + err.Error())
				return core.ResetToRoot()
			}
		}
		// A non-github url has no archive to remove; fall through to the entry removal.
	}

	globalPath, err := addon.GlobalListPath()
	if err == nil {
		err = addon.RemoveEntry(globalPath, g.name)
	}
	if err != nil {
		sh.SetStatus("error: " + err.Error())
		return core.ResetToRoot()
	}
	return core.Refresh(appctx.Global, true, "removed "+g.name)
}

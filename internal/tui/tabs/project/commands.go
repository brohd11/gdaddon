package project

import (
	"gdaddon/internal/tui/appctx"
	"strings"

	"github.com/brohd11/bubblestack/core"

	"gdaddon/internal/addon"

	tea "github.com/charmbracelet/bubbletea"
)

// finishInstallCmd pins the freshly installed url, resolved path, and version into the
// manifest (disk IO, so it stays an async cmd), then returns ProjectDirty as its result
// message; the router broadcasts it (reload + focus) when it arrives.
func finishInstallCmd(sh *core.Shared, selected addon.Addon, pick versionItem, instPath, instVersion string) tea.Cmd {
	manifestPath := appctx.Of(sh).ManifestPath
	name, url := selected.Name, pick.asset.URL
	// Installing from the local archive must not pin the machine-specific archive
	// path as the manifest url — keep the entry's canonical repo url instead.
	if pick.archived {
		url = ""
	}
	version := instVersion
	if version == "" {
		version = strings.TrimPrefix(pick.tag, "v")
	}
	// Branch-HEAD installs carry the branch name in pick.tag but have no release
	// tag; pick.branch marks them so we don't record a bogus tag.
	tag := pick.tag
	if pick.branch {
		tag = ""
	}

	status := "updated " + name + " → " + version
	return func() tea.Msg {
		_ = addon.UpdateEntry(manifestPath, name, url, instPath, version, tag)
		return core.Seq(
			core.SetStatus(status),
			core.PropagateAll(appctx.ProjectDirty{}),
			core.ShowTab(appctx.TitleProject),
		)
	}
}

// commitRemove removes the addon from the project: the installed files too when
// the chosen mode is "project + local", then the manifest entry. On success it
// broadcasts ProjectDirty, which reloads the browse list from the manifest and focuses it.
func commitRemove(sh *core.Shared, st addon.Status, mode int) core.Action {
	c := appctx.Of(sh)
	if mode == removeProjectLocal {
		if err := addon.Uninstall(st.Addon, c.ProjectRoot); err != nil {
			return core.Seq(
				core.SetStatusAndLog("error: "+err.Error()),
				core.ResetToRoot(),
			)
		}
	}
	if err := addon.RemoveEntry(c.ManifestPath, st.Addon.Name); err != nil {
		return core.Seq(
			core.SetStatusAndLog("error: "+err.Error()),
			core.ResetToRoot(),
		)
	}
	return core.Seq(
		core.SetStatus("removed "+st.Addon.Name),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.ShowTab(appctx.TitleProject),
	)
}

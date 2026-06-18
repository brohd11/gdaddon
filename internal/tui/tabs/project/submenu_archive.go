package project

import (
	"path"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/flows/packages"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newArchiveSubmenu builds the Archive command submenu for an installed addon: archive
// the currently installed version, or browse the repo (all releases + archived, plus
// HEAD) and archive any version via the shared packages flow.
func newArchiveSubmenu(st addon.Status, sh *core.Shared) *components.PickerScreen {
	items := []list.Item{}
	if st.Present() {
		items = append(items, components.Item{
			Name: "Current Version - " + st.LocalVersion,
			Desc: "save a local copy of the installed version",
			Pick: func(sh *core.Shared) core.Action { return archiveCurrentVersion(sh, st) },
		},
		)
	}
	items = append(items, components.Item{
		Name: "Browse repo",
		Desc: "pick any version or branch to archive",
		Pick: func(sh *core.Shared) core.Action {
			return core.Push(packages.BrowseRepo(st.Addon.URL, packages.BrowseOpts{
				Source:       packages.SourceAll,
				IncludeHEAD:  true,
				Endpoint:     packages.ArchiveEndpoint,
				MarkArchived: true,
			}))
		},
	})

	return components.NewPicker(items, components.PickerOpts{
		Title:   core.HeaderTitle(st.Addon.Name, st.LocalVersion, "Archive"),
		PopStop: true, // command hub: the browse/archive sub-flow returns here (PopTo)
	})
}

// archiveCurrentVersion archives the installed version by feeding the manifest
// url + local version through the shared archive confirm (download + store under
// repo/version). It reuses buildArchiveConfirm so the confirm body, repoID
// resolution, already-archived check, and task wiring stay in one place.
func archiveCurrentVersion(sh *core.Shared, st addon.Status) core.Action {
	if st.LocalVersion == "" {
		return core.SetStatusAndLog("cannot archive: installed version unknown")
	}
	asset := source.Asset{Name: archiveAssetName(st.Addon.URL), URL: st.Addon.URL}
	sel := versionItem{tag: st.LocalVersion, asset: asset}
	cs, status, ok := buildArchiveConfirm(st.Addon, st.LocalVersion, sel)
	if !ok {
		return core.SetStatusAndLog(status)
		// return core.Action{}
	}
	return core.Seq(
		core.SetStatusAndLog(status),
		core.Push(cs),
	)
}

// archiveAssetName derives a stored filename from the manifest url (e.g.
// "myaddon.zip" or "v1.2.3.zip"), falling back to a generic name.
func archiveAssetName(url string) string {
	name := path.Base(url)
	if name == "" || name == "." || name == "/" {
		return "package.zip"
	}
	return name
}

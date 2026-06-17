package project

import (
	"path"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// newArchiveSubmenu builds the Archive command submenu for an installed addon.
// For now it offers only the currently installed version; a search/browse option
// may follow as another row.
func newArchiveSubmenu(st addon.Status) *components.PickerScreen {
	items := []list.Item{
		components.Item{
			Name: "Current Version - " + st.LocalVersion,
			Desc: "save a local copy of the installed version",
			Pick: func(sh *core.Shared) tea.Cmd { return archiveCurrentVersion(sh, st) },
		},
	}
	return components.NewPicker(items, components.PickerOpts{
		Title: core.HeaderTitle(st.Addon.Name, st.LocalVersion, "Archive"),
	})
}

// archiveCurrentVersion archives the installed version by feeding the manifest
// url + local version through the shared archive confirm (download + store under
// repo/version). It reuses buildArchiveConfirm so the confirm body, repoID
// resolution, already-archived check, and task wiring stay in one place.
func archiveCurrentVersion(sh *core.Shared, st addon.Status) tea.Cmd {
	if st.LocalVersion == "" {
		sh.StatusMsg = "cannot archive: installed version unknown"
		return nil
	}
	asset := source.Asset{Name: archiveAssetName(st.Addon.URL), URL: st.Addon.URL}
	sel := versionItem{tag: st.LocalVersion, asset: asset}
	cs, status, ok := buildArchiveConfirm(st.Addon, st.LocalVersion, sel)
	if status != "" {
		sh.StatusMsg = status
	}
	if !ok {
		return nil
	}
	return core.Push(cs)
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

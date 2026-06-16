package project

import (
	"path"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// archiveMenuKind identifies a row in an addon's Archive submenu.
type archiveMenuKind int

const (
	archiveCurrent archiveMenuKind = iota
)

// archiveMenuItem is one command row in the Archive submenu.
type archiveMenuItem struct {
	title string
	desc  string
	kind  archiveMenuKind
}

func (i archiveMenuItem) Title() string       { return i.title }
func (i archiveMenuItem) FilterValue() string { return i.title }
func (i archiveMenuItem) Description() string { return i.desc }

// newArchiveSubmenu builds the Archive command submenu for an installed addon.
// For now it offers only the currently installed version; a search/browse option
// may follow.
func newArchiveSubmenu(st addon.Status) *components.PickerScreen {
	items := []list.Item{
		archiveMenuItem{
			title: "Current Version - " + st.LocalVersion,
			desc:  "save a local copy of the installed version",
			kind:  archiveCurrent,
		},
	}
	return components.NewPicker(items, components.PickerOpts{
		Title: core.HeaderTitle(st.Addon.Name, st.LocalVersion, "Archive"),
		OnSelect: func(sh *core.Shared, it list.Item) tea.Cmd {
			m, ok := it.(archiveMenuItem)
			if !ok {
				return nil
			}
			switch m.kind {
			case archiveCurrent:
				return archiveCurrentVersion(sh, st)
			}
			return nil
		},
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

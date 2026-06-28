package actions

import (
	"os"
	"path/filepath"

	"gdaddon/internal/archive"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/sysopen"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// Refreshes all lists and paths
func refreshAll() core.Action {
	return core.Seq(
		core.PropagateAll(appctx.ArchiveDirty{}),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.PropagateAll(appctx.GlobalDirty{}),
		core.PropagateAll(appctx.PathRefresh{}),
	)
}

// newPathsPicker is a quick navigation helper (Actions ▸ Paths): each row reveals a
// project-relevant location in the OS file manager. Rows are built from the live
// context, so missing locations (no manifest yet) are simply omitted. Selecting a row
// fires the open command asynchronously and leaves the picker open, so several spots
// can be opened in a row.
func newPathsPicker(sh *core.Shared) core.Screen {
	c := appctx.Of(sh)
	var items []list.Item

	add := func(name, path string, reveal bool) {
		if path == "" {
			return
		}
		items = append(items, components.Item{
			Name: name,
			Desc: path,
			Pick: func(sh *core.Shared) core.Action { return sysopen.Path(path, reveal) },
		})
	}

	add("Project", c.ProjectRoot, false)
	if c.ProjectRoot != "" {
		add("Addons Dir", filepath.Join(c.ProjectRoot, "addons"), false)
	}
	add("Manifest", c.ManifestPath, true)
	if home, err := os.UserHomeDir(); err == nil {
		add(".gdaddon", filepath.Join(home, ".gdaddon"), false)
	}
	if dir, err := archive.Dir(); err == nil {
		add("Archive", dir, false)
	}

	return components.NewPicker(items, components.PickerOpts{Crumb: "Paths"})
}

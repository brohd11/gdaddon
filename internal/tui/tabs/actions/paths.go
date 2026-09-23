package actions

import (
	"os"
	"path/filepath"

	"github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/sysopen"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/list"
)

// newPathsPicker (Actions ▸ Paths) reveals project locations in the file manager, omitting
// missing ones. The picker stays open so several can be opened.
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

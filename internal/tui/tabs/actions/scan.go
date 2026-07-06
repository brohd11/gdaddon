package actions

import (
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/newplugin"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newScanPicker scans the project for installed plugin folders not tracked in the
// manifest and lists them; selecting one opens the Track form (path/name prefilled,
// url suggested when an existing pathless entry matches) to start tracking it.
// Scanning is filesystem-only, so the picker is built synchronously.
//
// The picker is a PopTo boundary (a command hub) and re-scans on ProjectDirty: after
// tracking a plugin, commitTrack broadcasts ProjectDirty (rebuilding the rows so the
// just-tracked plugin drops off) then PopTo lands back here, ready to track the rest.
func newScanPicker(sh *core.Shared) *components.PickerScreen {
	return components.NewPicker(scanItems(sh), components.PickerOpts{
		Crumb:   "Scan",
		Title:   "Untracked plugins",
		PopStop: true,
		Refresh: func(sh *core.Shared, payload any) ([]list.Item, bool) {
			if _, ok := payload.(appctx.ProjectDirty); ok {
				return scanItems(sh), true
			}
			return nil, false
		},
	})
}

// scanItems builds the untracked-plugin rows from a fresh filesystem scan against the
// current manifest, falling back to a placeholder row when everything is tracked.
func scanItems(sh *core.Shared) []list.Item {
	c := appctx.Of(sh)
	found, _ := addon.UntrackedInstalls(c.ManifestPath, c.ProjectRoot)

	var items []list.Item
	for _, in := range found {
		in := in // capture per row
		desc := in.Path
		if in.Version != "" {
			desc += " · v" + in.Version
		}
		items = append(items, components.Item{
			Name: in.Name,
			Desc: desc,
			Pick: func(sh *core.Shared) core.Action {
				return core.Push(newplugin.NewFromInstall(in.Path, in.Name, in.Version, in.SuggestedURL, in.Kind, in.Branch))
			},
		})
	}
	items = components.EnsurePlaceholder(items, "(all installed plugins are tracked)", fmt.Sprintf("nothing untracked under %s", c.ProjectRoot))
	return items
}

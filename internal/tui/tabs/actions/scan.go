package actions

import (
	"fmt"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/newplugin"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/list"
)

// newScanPicker lists installed plugin folders the manifest does not track; picking one
// opens the Track form. It is a PopTo hub that re-scans on ProjectDirty, so tracked
// plugins drop off and you return here for the rest.
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

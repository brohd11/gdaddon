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
func newScanPicker(sh *core.Shared) *components.PickerScreen {
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
	if len(items) == 0 {
		items = append(items, components.Item{
			Name: "(all installed plugins are tracked)",
			Desc: fmt.Sprintf("nothing untracked under %s", c.ProjectRoot),
		})
	}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Scan", Title: "Untracked plugins"})
}

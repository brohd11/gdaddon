package sets

import (
	"fmt"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// newImportConfirm confirms importing a set into the project manifest.
func newImportConfirm(setName string) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb: "Import",
		Text:  fmt.Sprintf("Import all plugins from set %q into the project?", setName),
		OnYesLambda: func(sh *core.Shared) core.Action {
			return importSetToProject(sh, setName)
		},
	})
}

// importSetToProject adds every set entry to the project manifest (deduplicated by repo),
// logs a summary and shows the Project tab.
func importSetToProject(sh *core.Shared, setName string) core.Action {
	c := appctx.Of(sh)
	if c.ManifestPath == "" {
		return core.SetStatusAndLog("no project manifest — create one first (Actions → Create manifest)")
	}
	setPath, err := addon.SetPath(setName)
	if err != nil {
		return core.StatusErr(err)
	}
	entries, err := addon.Parse(setPath)
	if err != nil {
		return core.StatusErr(err)
	}
	added, skipped := 0, 0
	for _, e := range entries {
		// e is a full addon.Addon, so AddEntryFull carries url/path/version/tag and
		// the clone flag straight through — a set's clone entries import as clones.
		if err := addon.AddEntryFull(c.ManifestPath, e); err != nil {
			skipped++
			continue
		}
		added++
	}
	// Log the summary (status line + output pane), then reload the Project tab and
	// jump there (ShowTab unwinds the stack, discarding the confirm).
	return core.Seq(
		core.SetStatusAndLog(
			fmt.Sprintf("imported set %s — %d added, %d skipped", setName, added, skipped),
			true, // forceShow: surface the summary in the output pane
		),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.ShowTab(appctx.TitleProject),
	)
}

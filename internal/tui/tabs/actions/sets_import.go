package actions

import (
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// importSetToProject adds every entry in the set to the project manifest (deduped by
// repo id, carrying any pinned version), then shows the Project tab reloaded.
func importSetToProject(sh *core.Shared, setName string) core.Action {
	c := appctx.Of(sh)
	if c.ManifestPath == "" {
		return core.SetStatusAndLog("no project manifest — create one first (Actions → Create manifest)")
	}
	setPath, err := addon.SetPath(setName)
	if err != nil {
		return core.SetStatusAndLog("error: " + err.Error())
	}
	entries, err := addon.Parse(setPath)
	if err != nil {
		return core.SetStatusAndLog("error: " + err.Error())
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
	// Acknowledge with a popup over the Set submenu; dismissing it reloads the
	// Project tab and jumps there (ShowTab unwinds the stack, discarding the popup).
	return core.Push(newImportDonePopup(setName, added, skipped))
}

// newImportDonePopup is the "job done" acknowledgement shown after an import: a small
// box summarizing the result; pressing done reloads and shows the Project tab.
func newImportDonePopup(setName string, added, skipped int) *components.DialogScreen {
	return &components.DialogScreen{
		Overlay: true, // a centered modal over the Set submenu
		Title:   "Import complete",
		Render: func(*core.Shared) string {
			return fmt.Sprintf("✓ %s\n\n%d added, %d skipped", setName, added, skipped)
		},
		OnYes: func(*core.Shared) core.Action {
			return core.Seq(
				core.PropagateAll(appctx.ProjectDirty{}),
				core.ShowTab(appctx.TitleProject),
			)
		},
		Help: components.DefaultPopupHelp,
	}
}

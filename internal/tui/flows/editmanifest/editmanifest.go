// Package editmanifest is the shared "Edit Manifest" flow: a form that lists an
// entry's raw fields (url, path, version, tag, clone) prefilled with their current
// values and writes them back. It works against any of the flat-shaped manifest
// files — the project manifest, the global list, or a set — so it lives in the flows
// layer (core ← components ← flows ← tabs ← tui) and is opened by more than one tab
// with the matching dirty payload.
//
// Blanking a text field clears that field (addon.EditEntry removes the line), the
// inverse of UpdateEntry's "blank leaves it untouched". clone is a bool toggle and
// is written separately via addon.SetCloneFlag.
package editmanifest

import (
	"strings"

	"gdaddon/internal/addon"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
)

// New builds the Edit Manifest form for entry a in the manifest at manifestPath.
// dirty is broadcast on a successful save (e.g. appctx.ProjectDirty{}) so whichever
// tab root owns this manifest reloads. The entry name is read-only. In global mode
// only url and path are shown — version, tag, and clone are irrelevant to the global
// library, so those fields (and the clone write) are omitted.
func New(manifestPath string, a addon.Addon, dirty any, globalMode bool) *components.FormScreen {
	urlF := components.NewTextField("url", "URL:     ", "(blank to clear)")
	pathF := components.NewTextField("path", "Path:    ", "(blank to clear)")
	urlF.SetValue(a.URL)
	pathF.SetValue(a.Path)

	fields := []components.FormField{
		components.NewHeading("Edit " + a.Name),
		components.NewSpacer(),
		urlF, pathF,
	}
	help := []key.Binding{
		core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
		core.Hint("save", core.Keys.Select),
		core.Hint("cancel", core.Keys.Back),
	}

	var versionF, tagF *components.TextField
	var cloneF *components.ToggleField
	if !globalMode {
		versionF = components.NewTextField("version", "Version: ", "(blank to clear)")
		tagF = components.NewTextField("tag", "Tag:     ", "(blank to clear)")
		versionF.SetValue(a.Version)
		tagF.SetValue(a.Tag)
		cloneF = components.NewToggleField("clone", "Clone:   ", []string{"false", "true"}, "|")
		if a.Clone {
			cloneF.OnToggle(true)
		}
		fields = append(fields, versionF, tagF, components.NewSpacer(), cloneF)
		help = []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint("clone", core.Keys.Left, core.Keys.Right),
			core.Hint("save", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		}
	}

	return components.NewForm(components.FormOpts{
		Crumb:  "Edit Manifest",
		Fields: fields,
		Focus:  "url",
		Help:   help,
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			url := strings.TrimSpace(f.Value("url"))
			path := strings.TrimSpace(f.Value("path"))
			version := strings.TrimSpace(f.Value("version"))
			tag := strings.TrimSpace(f.Value("tag"))

			if err := addon.EditEntry(manifestPath, a.Name, url, path, version, tag); err != nil {
				return core.Seq(core.SetStatusAndLog("error: "+err.Error()), core.Async(f.Focus("url")))
			}
			if !globalMode {
				if err := addon.SetCloneFlag(manifestPath, a.Name, cloneF.Index() == 1); err != nil {
					return core.Seq(core.SetStatusAndLog("error: "+err.Error()), core.Async(f.Focus("url")))
				}
			}
			return core.Seq(
				core.SetStatusAndLog(a.Name+": updated"),
				core.PropagateAll(dirty),
				core.Pop(),
			)
		},
	})
}

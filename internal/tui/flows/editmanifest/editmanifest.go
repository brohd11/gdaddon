// Package editmanifest is the shared "Edit Manifest" flow: a form that lists an
// entry's raw fields (url, path, version, tag, kind) prefilled with their current
// values and writes them back. It works against any of the flat-shaped manifest
// files — the project manifest, the global list, or a set — so it lives in the flows
// layer (core ← components ← flows ← tabs ← tui) and is opened by more than one tab
// with the matching dirty payload.
//
// Blanking a text field clears that field (addon.EditEntry removes the line), the
// inverse of UpdateEntry's "blank leaves it untouched". kind is a 3-way toggle
// (package/clone/submodule) and is written separately via addon.SetKind.
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
// only url and path are shown — version, tag, and kind are irrelevant to the global
// library, so those fields (and the kind write) are omitted.
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
	var kindF *components.ToggleField
	if !globalMode {
		versionF = components.NewTextField("version", "Version: ", "(blank to clear)")
		tagF = components.NewTextField("tag", "Tag:     ", "(blank to clear)")
		versionF.SetValue(a.Version)
		tagF.SetValue(a.Tag)
		kindF = components.NewToggleField("kind", "Kind:    ", addon.KindOptions, "|")
		kindF.SetIndex(addon.KindIndex(a.Kind))
		fields = append(fields, versionF, tagF, components.NewSpacer(), kindF)
		help = []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint("kind", core.Keys.Left, core.Keys.Right),
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
				return core.SeqErr(err, core.Async(f.Focus("url")))
			}
			if !globalMode {
				if err := addon.SetKind(manifestPath, a.Name, addon.ParseKind(kindF.Value())); err != nil {
					return core.SeqErr(err, core.Async(f.Focus("url")))
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

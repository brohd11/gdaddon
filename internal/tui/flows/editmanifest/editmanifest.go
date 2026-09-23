// Package editmanifest is the Edit Manifest flow: a form of an entry's raw fields (name,
// url, path, version, tag, kind) written back to any flat manifest (project, global list,
// or set). Blanking a field removes it (addon.EditEntry); kind is written with
// addon.SetKind.
package editmanifest

import (
	"strings"

	"github.com/brohd11/gdaddon/internal/addon"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
)

// New builds the Edit Manifest form for entry a. dirty is broadcast on save so the owning
// tab reloads. The key is read-only (it is the entry's identity); the name is the editable
// label. Global mode shows only name, url and path.
func New(manifestPath string, a addon.Addon, dirty any, globalMode bool) *components.FormScreen {
	nameF := components.NewTextField("name", "Name:    ", "(blank to clear)")
	urlF := components.NewTextField("url", "URL:     ", "(blank to clear)")
	pathF := components.NewTextField("path", "Path:    ", "(blank to clear)")
	nameF.SetValue(a.Display)
	urlF.SetValue(a.URL)
	pathF.SetValue(a.Path)

	fields := []components.FormField{
		// The heading carries the key, not the label: it is what every write below
		// addresses and the one thing this form cannot change.
		components.NewHeading("Edit " + a.Name),
		components.NewSpacer(),
		nameF, urlF, pathF,
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
			display := strings.TrimSpace(f.Value("name"))
			url := strings.TrimSpace(f.Value("url"))
			path := strings.TrimSpace(f.Value("path"))
			version := strings.TrimSpace(f.Value("version"))
			tag := strings.TrimSpace(f.Value("tag"))

			if err := addon.EditEntry(manifestPath, a.Name, url, path, version, tag); err != nil {
				return core.SeqErr(err, core.Async(f.Focus("url")))
			}
			// Blank clears the line, matching every other field on this form.
			if err := addon.SetDisplayName(manifestPath, a.Name, display); err != nil {
				return core.SeqErr(err, core.Async(f.Focus("name")))
			}
			if !globalMode {
				if err := addon.SetKind(manifestPath, a.Name, addon.ParseKind(kindF.Value())); err != nil {
					return core.SeqErr(err, core.Async(f.Focus("url")))
				}
			}
			return core.Seq(
				core.SetStatusAndLog(a.Name+": updated"),
				core.PropagateAll(dirty),
				// Pop the parent submenu too: its closures hold the pre-edit entry, so reopening rebuilds
				// it from the new manifest.
				core.Pop(2),
			)
		},
	})
}

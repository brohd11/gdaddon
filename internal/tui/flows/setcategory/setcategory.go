// Package setcategory is the Set Category flow: a one-field form that writes an entry's
// category to a manifest (the global list, or a project manifest where it overrides the
// global one). It also owns the category input Edit Manifest reuses.
package setcategory

import (
	"github.com/brohd11/gdaddon/internal/addon"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
)

// PlaceholderClear is the hint where blank means no category (the global list).
const PlaceholderClear = "e.g. tools/editor (blank to clear)"

// PlaceholderInherit is the hint where blank falls back to the global category.
func PlaceholderInherit(inherited string) string {
	if inherited == "" {
		return "e.g. tools/editor (blank uses the global one)"
	}
	return "blank uses global: " + inherited
}

// Field is the shared category input (key "category"), pre-filled with current.
func Field(current, placeholder string) *components.TextField {
	f := components.NewTextField("category", "Category: ", placeholder)
	f.SetValue(current)
	return f
}

// Opts configures New.
type Opts struct {
	ManifestPath string // the manifest to write (global list or project manifest)
	Name         string // the entry key every write addresses
	Label        string // heading and status text
	Current      string // the entry's own category
	// Inherited is what a blank value falls back to (a project entry's global category).
	// Empty for the global list, where blank means none.
	Inherited string
	// Inherits marks a manifest whose blank falls back to the global category (a project),
	// even when that is currently empty.
	Inherits bool
	Dirty    any // broadcast on save so the owning tab reloads
}

// New builds the form. Saving writes the normalized category (blank removes the line),
// broadcasts Dirty and pops twice: the calling submenu's closures hold the old entry.
func New(o Opts) *components.FormScreen {
	placeholder := PlaceholderClear
	if o.Inherits {
		placeholder = PlaceholderInherit(o.Inherited)
	}
	return components.NewForm(components.FormOpts{
		Crumb: "Set Category",
		Fields: []components.FormField{
			components.NewHeading("Category for " + o.Label),
			components.NewSpacer(),
			Field(o.Current, placeholder),
		},
		Help: []key.Binding{
			core.Hint("save", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		},
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			category := addon.NormalizeCategory(f.Value("category"))
			if err := addon.SetCategory(o.ManifestPath, o.Name, category); err != nil {
				return core.SeqErr(err, core.Async(f.Focus("category")))
			}
			return core.Seq(
				core.SetStatusAndLog(o.Label+": "+statusText(category, o)),
				core.PropagateAll(o.Dirty),
				core.Pop(2),
			)
		},
	})
}

// statusText describes the saved state: the category, the global one now used, or none.
func statusText(category string, o Opts) string {
	switch {
	case category != "":
		return "category " + category
	case o.Inherits && o.Inherited != "":
		return "using global category " + o.Inherited
	default:
		return "category cleared"
	}
}

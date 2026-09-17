package appctx

import (
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// ListDensity shares the session preference with all standard roots and pickers.
func (c *Ctx) ListDensity() *bool { return &c.Compact }

// RootListOpts supplies the app key binding; density is resolved through ListDensity.
func RootListOpts(_ *core.Shared, title string) components.RootListOpts {
	return components.RootListOpts{
		PickerOpts: components.PickerOpts{Title: title, DensityKey: AppKeys.Density},
	}
}

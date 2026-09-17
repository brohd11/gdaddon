package appctx

import (
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// RootListOpts connects every tab list to the session's shared density preference.
// The component owns the toggle, help entry and synchronization during sizing.
func RootListOpts(sh *core.Shared, title string) components.RootListOpts {
	return components.RootListOpts{
		PickerOpts:   components.PickerOpts{Title: title, DensityKey: AppKeys.Density},
		CompactState: &Of(sh).Compact,
	}
}

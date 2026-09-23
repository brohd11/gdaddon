package actions

import (
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// newSelfUpdateLoading starts Actions ▸ Update gdaddon (the shared flow), using the same
// hooks as the startup check (appctx.SelfUpdateHooks).
func newSelfUpdateLoading(sh *core.Shared) *components.LoadingScreen {
	return components.NewSelfUpdateLoading(appctx.SelfUpdateHooks(appctx.Of(sh).Version))
}

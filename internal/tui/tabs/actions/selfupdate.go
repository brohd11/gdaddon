package actions

import (
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// newSelfUpdateLoading is the entry point of the Actions ▸ Update gdaddon flow:
// loading → confirm → task. The flow itself is shared (bubblestack/components); what
// stays gdaddon-specific is the hook set — app name, running version, and the
// internal/selfupdate mechanism — built once in appctx so the startup check and this
// screen wire the same operations.
func newSelfUpdateLoading(sh *core.Shared) *components.LoadingScreen {
	return components.NewSelfUpdateLoading(appctx.SelfUpdateHooks(appctx.Of(sh).Version))
}

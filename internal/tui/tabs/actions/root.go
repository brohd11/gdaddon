package actions

import (
	"charm.land/bubbles/v2/list"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
)

// NewActionsScreen builds the Actions tab. PathRefresh updates the manifest actions
// without grabbing focus from the Project tab.
func NewActionsScreen(sh *core.Shared) *components.RootListScreen {
	opts := appctx.RootListOpts(sh, "Actions")
	opts.Refresh = func(sh *core.Shared, payload any) ([]list.Item, bool) {
		if _, ok := payload.(appctx.PathRefresh); ok {
			return actionItems(sh), true
		}
		return nil, false
	}
	return components.NewRootList(actionItems(sh), opts)
}

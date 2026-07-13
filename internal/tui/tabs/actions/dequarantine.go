package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gdaddon/internal/quarantine"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// newDequarantineConfirm is the macOS-only Actions ▸ Dequarantine Addons flow:
// compiled plugins arrive tagged with com.apple.quarantine, which Gatekeeper uses to
// block their native binaries from loading. Clearing it is destructive-free but
// deliberate, so it sits behind a confirm before the task walks the tree.
func newDequarantineConfirm(sh *core.Shared) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb: "Dequarantine",
		Text:  "Remove com.apple.quarantine from the addons folder?",
		OnYes: core.Push(newDequarantineTask()),
	})
}

// newDequarantineTask clears com.apple.quarantine over <root>/addons via
// quarantine.Clear, which prunes hidden dirs (an addon's .git holds read-only objects
// that only ever answer with permission errors) and reports counts rather than a line
// per file. Entries that refuse the removal are summarized, not treated as a hard
// failure, so a partially-clean tree still reads as done.
func newDequarantineTask() *components.TaskScreen {
	cleared := 0
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		defer func() { done <- core.TaskEvent{Done: true} }()

		addonsPath := filepath.Join(appctx.Of(sh).ProjectRoot, "addons")
		if _, err := os.Stat(addonsPath); err != nil {
			report("no addons folder at %s — nothing to do", addonsPath)
			return
		}

		report("clearing com.apple.quarantine on %s …", addonsPath)
		res, err := quarantine.Clear(ctx, addonsPath)
		cleared = res.Cleared
		if err != nil {
			report("stopped: %v", err)
			return
		}
		report("cleared %d file(s)", res.Cleared)
		if res.Denied > 0 {
			report("%d path(s) skipped (permission denied)", res.Denied)
		}
		for _, e := range res.Errs {
			report("failed: %s", e)
		}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		return core.SetStatusAndLog(fmt.Sprintf("quarantine cleared from %d file(s)", cleared))
	}
	onDismiss := func(sh *core.Shared) core.Action {
		return core.PopTo() // back to the Actions menu
	}
	return components.NewStayTask("dequarantining addons…", "done — esc to go back", run, onDone, onDismiss)
}

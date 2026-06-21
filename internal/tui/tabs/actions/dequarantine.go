package actions

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// newDequarantineConfirm is the macOS-only Actions ▸ Dequarantine Addons flow:
// compiled plugins arrive tagged with com.apple.quarantine, which Gatekeeper uses to
// block their native binaries from loading. Clearing it is destructive-free but
// deliberate, so it sits behind a confirm before the task runs xattr.
func newDequarantineConfirm(sh *core.Shared) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb: "Dequarantine",
		Text:  "Remove com.apple.quarantine from the addons folder?",
		OnYes: core.Push(newDequarantineTask()),
	})
}

// newDequarantineTask clears com.apple.quarantine recursively over <root>/addons.
// xattr -r already walks the whole tree, so one invocation covers every file; a
// non-zero exit (e.g. the attribute absent on some files) is reported as info rather
// than treated as a hard failure, so a partially-clean tree still reads as done.
func newDequarantineTask() *components.TaskScreen {
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		defer func() { done <- core.TaskEvent{Done: true} }()

		addonsPath := filepath.Join(appctx.Of(sh).ProjectRoot, "addons")
		if _, err := os.Stat(addonsPath); err != nil {
			report("no addons folder at %s — nothing to do", addonsPath)
			return
		}

		report("clearing com.apple.quarantine on %s …", addonsPath)
		cmd := exec.CommandContext(ctx, "xattr", "-dr", "com.apple.quarantine", addonsPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			report("xattr finished with issues (some files may have lacked the attribute):\n%s", string(out))
			return
		}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		return core.SetStatusAndLog("quarantine cleared")
	}
	onDismiss := func(sh *core.Shared) core.Action {
		return core.PopTo() // back to the Actions menu
	}
	return components.NewStayTask("dequarantining addons…", "done — esc to go back", run, onDone, onDismiss)
}

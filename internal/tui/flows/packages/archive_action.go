package packages

import (
	"context"
	"fmt"
	"strings"

	arch "github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/list"
)

// ArchiveEndpoint is an Endpoint that downloads and archives the chosen asset (reporting
// when it is already archived). Used by the Global and Project archive flows.
func ArchiveEndpoint(sel Selection) core.Screen {
	items := []list.Item{
		components.Item{
			Name: fmt.Sprintf("⬇ Add to archive - %s", sel.Asset.Name),
			Desc: "save a local copy of this package",
			Pick: func(sh *core.Shared) core.Action {
				cs, status, ok := NewArchiveConfirm(sel.RepoID, sel.RepoID, sel.Tag, []source.Asset{sel.Asset})
				if !ok {
					return core.SetStatusAndLog(status)
				}
				return core.Push(cs)
			},
		},
	}
	return components.NewPicker(items, components.PickerOpts{
		Crumb: "Package",
		Title: sel.RepoID,
	})
}

// NewArchiveConfirm builds the confirm that downloads assets and stores them under
// repoID/tag. ok is false (with a status) when nothing remains after dropping local
// assets.
func NewArchiveConfirm(name, repoID, tag string, assets []source.Asset) (*components.DialogScreen, string, bool) {
	// Drop already-archived (local) assets; nothing to fetch for those.
	var remote []source.Asset
	for _, a := range assets {
		if !isArchived(a) {
			remote = append(remote, a)
		}
	}
	if len(remote) == 0 {
		return nil, tag + " already archived", false
	}
	cs := components.CreateConfirmScreen(components.ConfirmSimple{
		Title: "Archive " + tag,
		Text:  archiveConfirmBody(name, tag, remote),
		OnYes: core.Replace(newArchiveTask(tag, repoID, remote)),
	})
	return cs, "", true
}

func archiveConfirmBody(name, tag string, assets []source.Asset) string {
	root, _ := arch.Dir()
	lines := make([]string, len(assets))
	for i, a := range assets {
		lines[i] = "    • " + strings.TrimSuffix(a.Name, archivedSuffix)
	}
	return fmt.Sprintf(
		"Archive %s\n\n  version:   %s\n  packages:\n%s\n\n  into:      %s",
		name, tag, strings.Join(lines, "\n"), root)
}

// newArchiveTask downloads and stores each asset, broadcasts ArchiveDirty, stays on the log,
// and returns to the nearest hub.
func newArchiveTask(tag, repoID string, assets []source.Asset) *components.TaskScreen {
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		for _, a := range assets {
			report("downloading %s …", strings.TrimSuffix(a.Name, archivedSuffix))
			if err := arch.Archive(ctx, repoID, tag, a); err != nil {
				done <- core.TaskEvent{Done: true, Err: err}
				return
			}
		}
		done <- core.TaskEvent{Done: true}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		if ev.Err != nil {
			return core.SetStatusAndLog("archive failed: " + ev.Err.Error())
		}
		return core.Seq(
			core.SetStatusAndLog("archived "+tag),
			core.PropagateAll(appctx.ArchiveDirty{}),
		)
	}
	onDismiss := func(sh *core.Shared) core.Action {
		return core.PopTo() // back to the command hub that opened this flow
	}
	return components.NewStayTask("archiving "+tag+"…", "done — esc to go back", run, onDone, onDismiss)
}

// isArchived reports whether an asset is a local (already-archived) copy rather than
// a remote URL to fetch.
func isArchived(a source.Asset) bool { return !strings.HasPrefix(a.URL, "http") }

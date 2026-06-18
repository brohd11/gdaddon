package packages

import (
	"fmt"
	"strings"

	arch "gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
)

// ArchiveEndpoint is a ready-made Endpoint that offers to download and archive the
// chosen version's asset (a no-op for an already-archived asset, in which case the
// confirm builder reports it instead). Shared by every browse flow whose leaf action
// is "save a local copy" (Global "Add to Archive", Project Archive → Browse repo).
func ArchiveEndpoint(repoID, tag string, asset source.Asset) *components.PickerScreen {
	items := []list.Item{
		components.Item{
			Name: "⬇ Add to archive",
			Desc: "save a local copy of this package",
			Pick: func(sh *core.Shared) core.Action {
				cs, status, ok := NewArchiveConfirm(repoID, repoID, tag, []source.Asset{asset})
				if !ok {
					return core.SetStatusAndLog(status)
				}
				return core.Push(cs)
			},
		},
	}
	return components.NewPicker(items, components.PickerOpts{Title: repoID + " - " + stripSuffix(asset.Name)})
}

var archiveConfirmHelp = []key.Binding{
	core.Hint("confirm", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

// NewArchiveConfirm builds the confirm that downloads the given assets and stores
// them under repoID/tag (a local copy that survives upstream delisting). It returns
// ok=false (with a status line) when there is nothing to archive — already-archived
// (local) assets are dropped first. name labels the package in the confirm/crumb.
// Shared by the project Archive command and the Global "Add to archive" flow.
func NewArchiveConfirm(name, repoID, tag string, assets []source.Asset) (*components.ConfirmScreen, string, bool) {
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

	cs := &components.ConfirmScreen{
		Crumb:  core.HeaderTitle(name, "", "Archive "+tag),
		Render: func(sh *core.Shared) string { return sh.Box(archiveConfirmBody(name, tag, remote)) },
		OnYes: func(sh *core.Shared) core.Action {
			return core.Replace(newArchiveTask(tag, repoID, remote))
		},
		Help: archiveConfirmHelp,
	}
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

// newArchiveTask downloads each asset and stores it under repo/tag, then broadcasts
// ArchiveDirty so the Archive tab reloads. It stays on the log until dismissed and
// pops back to the nearest command hub (PopTo).
func newArchiveTask(tag, repoID string, assets []source.Asset) *components.TaskScreen {
	run := func(sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		for _, a := range assets {
			report("downloading %s …", strings.TrimSuffix(a.Name, archivedSuffix))
			if err := arch.Archive(repoID, tag, a); err != nil {
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

func stripSuffix(s string) string {
	s = strings.TrimSuffix(s, archivedSuffix)
	s = strings.TrimSuffix(s, archivedMarker)
	s = strings.TrimSpace(s)
	return s
}

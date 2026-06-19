package project

import (
	"fmt"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/flows/packages"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// The confirm box mechanism lives in components.ConfirmScreen; the builders below
// supply its crumb/render/onYes closures for each feature (install / archive / new
// plugin). confirmHelp and newPluginConfirmHelp are the per-builder key hints.

var confirmHelp = []key.Binding{
	core.Hint("confirm", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

// ---------- install confirm ----------

// installEndpoint adapts the install confirm into a packages.Endpoint: it captures the
// selected addon and its installed version, and builds the confirm for whichever
// release/branch/asset the shared browse flow hands back.
func installEndpoint(selected addon.Addon, local string) packages.Endpoint {
	return func(sel packages.Selection) core.Screen {
		pick := versionItem{tag: sel.Tag, asset: sel.Asset, archivedAsset: sel.ArchivedAsset, branch: sel.Branch, archived: sel.Archived}
		return newInstallConfirm(selected, local, pick)
	}
}

// install source modes (also the vertical option order); shown only when the picked
// version has a local archived copy to install from.
const (
	installDownload = iota // fetch from the remote url
	installArchive         // install from the local archived copy
)

var installToggleHelp = []key.Binding{
	core.Hint("source", core.Keys.Up, core.Keys.Down),
	core.Hint("confirm", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

func newInstallConfirm(selected addon.Addon, local string, pick versionItem) *components.ConfirmScreen {
	// No archived copy ⇒ the plain confirm (current behavior).
	if pick.archivedAsset.URL == "" {
		return &components.ConfirmScreen{
			Crumb:  core.HeaderTitle(selected.Name, local, pickSection(pick)),
			Render: func(sh *core.Shared) string { return sh.Box(confirmInstallBody(sh, selected, pick)) },
			OnYes: func(sh *core.Shared) core.Action {
				return core.Replace(newInstallTask(selected, local, pick))
			},
			Help: confirmHelp,
		}
	}
	// Archived copy exists ⇒ offer a Download/Archive source toggle (default Download).
	mode := installDownload
	return &components.ConfirmScreen{
		Crumb: core.HeaderTitle(selected.Name, local, pickSection(pick)),
		Render: func(sh *core.Shared) string {
			body := confirmInstallBody(sh, selected, effectivePick(pick, mode))
			return sh.Box(body + "\n\n  source:\n" + installSourceOptions(mode))
		},
		OnKey: func(sh *core.Shared, k string) core.Action {
			switch {
			case core.MatchKey(k, core.Keys.Up):
				if mode > installDownload {
					mode--
				}
			case core.MatchKey(k, core.Keys.Down):
				if mode < installArchive {
					mode++
				}
			}
			return core.Action{}
		},
		OnYes: func(sh *core.Shared) core.Action {
			return core.Replace(newInstallTask(selected, local, effectivePick(pick, mode)))
		},
		Help: installToggleHelp,
	}
}

// effectivePick resolves the source toggle: in archive mode it installs from the local
// copy (swapping the url and flagging archived so finishInstallCmd keeps the canonical
// manifest url); the asset name stays the remote one for clean crumbs/labels.
func effectivePick(pick versionItem, mode int) versionItem {
	if mode == installArchive && pick.archivedAsset.URL != "" {
		pick.asset.URL = pick.archivedAsset.URL
		pick.archived = true
	}
	return pick
}

func confirmInstallBody(sh *core.Shared, selected addon.Addon, pick versionItem) string {
	// Hard-wrap the (space-less) URL to fit inside the box.
	urlBlock := core.IndentLines(core.HardWrap(pick.asset.URL, sh.ConfirmWidth()-4), "    ")
	return fmt.Sprintf(
		"Install %s\n\n  version:  %s\n  asset:    %s\n  path:     %s\n  url:\n%s",
		selected.Name, pick.tag, pick.asset.Name, selected.Path, urlBlock)
}

// installSourceOptions renders the two install sources stacked vertically, the active
// one marked and highlighted (mirrors removeOptions).
func installSourceOptions(mode int) string {
	active := lipgloss.NewStyle().Foreground(core.FocusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(core.MutedColor)
	opts := []struct{ label, desc string }{
		{"Download", "fetch a fresh copy from the remote"},
		{"Archive", "install from the local archived copy"},
	}
	lines := make([]string, len(opts))
	for i, o := range opts {
		text := o.label + " — " + o.desc
		if i == mode {
			lines[i] = "  ▸ " + active.Render(text)
		} else {
			lines[i] = "    " + dim.Render(text)
		}
	}
	return strings.Join(lines, "\n")
}

// ---------- remove confirm ----------

// remove modes (also the vertical option order).
const (
	removeProject      = iota // remove the manifest entry only
	removeProjectLocal        // also delete the installed files
)

var removeConfirmHelp = []key.Binding{
	core.Hint("option", core.Keys.Up, core.Keys.Down),
	core.Hint("remove", core.Keys.Select),
	core.Hint("cancel", core.Keys.Back),
}

// newRemoveConfirm builds the project Remove confirm: a vertical selector between
// removing just the manifest entry or that plus the installed files. ↑/↓ move the
// selection (via the confirm's OnKey), enter commits the chosen mode.
func newRemoveConfirm(st addon.Status) *components.ConfirmScreen {
	mode := removeProject // local copy the selector mutates; default = non-destructive
	return &components.ConfirmScreen{
		Crumb:  core.HeaderTitle(st.Addon.Name, st.LocalVersion, "Remove"),
		Render: func(sh *core.Shared) string { return sh.Box(removeConfirmBody(sh, st, mode)) },
		OnKey: func(sh *core.Shared, k string) core.Action {
			switch {
			case core.MatchKey(k, core.Keys.Up):
				if mode > removeProject {
					mode--
				}
			case core.MatchKey(k, core.Keys.Down):
				if mode < removeProjectLocal {
					mode++
				}
			}
			return core.Action{}
		},
		OnYes: func(sh *core.Shared) core.Action { return commitRemove(sh, st, mode) },
		Help:  removeConfirmHelp,
	}
}

func removeConfirmBody(sh *core.Shared, st addon.Status, mode int) string {
	path := st.Addon.Path
	if path == "" {
		path = "(none)"
	}
	return fmt.Sprintf("Remove %s\n\n  path:  %s\n\n%s", st.Addon.Name, path, removeOptions(mode))
}

// removeOptions renders the two removal modes stacked vertically, the active one
// marked and highlighted (vertical analog of the New Plugin target toggle).
func removeOptions(mode int) string {
	active := lipgloss.NewStyle().Foreground(core.FocusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(core.MutedColor)
	opts := []struct{ label, desc string }{
		{"Project", "remove from the project manifest only"},
		{"Project + local files", "also delete the installed files"},
	}
	lines := make([]string, len(opts))
	for i, o := range opts {
		text := o.label + " — " + o.desc
		if i == mode {
			lines[i] = "  ▸ " + active.Render(text)
		} else {
			lines[i] = "    " + dim.Render(text)
		}
	}
	return strings.Join(lines, "\n")
}

// ---------- archive confirm ----------

// buildArchiveConfirm resolves the repo id and hands the chosen version's asset to
// the shared packages.NewArchiveConfirm (which owns the confirm body, already-archived
// check, and download task). It returns ok=false (with a status line) when there is
// nothing to archive: an error or an already-archived selection.
func buildArchiveConfirm(selected addon.Addon, local string, pick versionItem) (*components.ConfirmScreen, string, bool) {
	repoID, err := source.RepoID(selected.URL)
	if err != nil {
		return nil, "cannot archive: " + err.Error(), false
	}
	return packages.NewArchiveConfirm(selected.Name, repoID, pick.tag, []source.Asset{pick.asset})
}

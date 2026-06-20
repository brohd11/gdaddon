package project

import (
	"fmt"
	"strings"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

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
		pick := versionItem{tag: sel.Tag, asset: sel.Asset, archivedAsset: sel.ArchivedAsset, repoID: sel.RepoID, branch: sel.Branch, archived: sel.Archived}
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

// branch install modes (also the vertical option order).
const (
	installPackageMode = iota // download + unzip the branch archive (current behavior)
	installCloneMode          // git clone the repo as a live working copy (keeps .git)
)

var installCloneToggleHelp = []key.Binding{
	core.Hint("mode", core.Keys.Up, core.Keys.Down),
	core.Hint("confirm", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

func newInstallConfirm(selected addon.Addon, local string, pick versionItem) *components.ConfirmScreen {
	// Branch (HEAD) installs offer a Package/Clone mode toggle: clone installs the
	// branch as a live git working copy for development (see cloneModeOptions).
	if pick.branch {
		mode := installPackageMode
		return &components.ConfirmScreen{
			Crumb: "Install",
			Render: func(sh *core.Shared) string {
				body := confirmInstallBody(sh, selected, pick)
				body += "\n\n  mode:\n" + cloneModeOptions(mode)
				if mode == installCloneMode {
					body += "\n\n" + cloneModeWarning(selected)
				}
				return sh.Box(body)
			},
			OnKey: func(sh *core.Shared, k string) core.Action {
				switch {
				case core.MatchKey(k, core.Keys.Up):
					if mode > installPackageMode {
						mode--
					}
				case core.MatchKey(k, core.Keys.Down):
					if mode < installCloneMode {
						mode++
					}
				}
				return core.Action{}
			},
			OnYes: func(sh *core.Shared) core.Action {
				p := pick
				p.clone = mode == installCloneMode
				return core.Replace(newInstallTask(selected, local, p))
			},
			Help: installCloneToggleHelp,
		}
	}
	// No archived copy ⇒ the plain confirm (current behavior).
	if pick.archivedAsset.URL == "" {
		return components.CreateConfirmScreen(components.ConfirmSimple{
			Render: func(sh *core.Shared) string { return sh.Box(confirmInstallBody(sh, selected, pick)) },
			OnYes:  core.Replace(newInstallTask(selected, local, pick)),
		})
		// return &components.ConfirmScreen{
		// 	Title:  pickSection(pick),
		// 	Crumb:  "Install",
		// 	Render: func(sh *core.Shared) string { return sh.Box(confirmInstallBody(sh, selected, pick)) },
		// 	OnYes: func(sh *core.Shared) core.Action {
		// 		return core.Replace(newInstallTask(selected, local, pick))
		// 	},
		// 	Help: confirmHelp,
		// }
	}
	// Archived copy exists ⇒ offer a Download/Archive source toggle (default Download).
	mode := installDownload
	return &components.ConfirmScreen{
		// Title: pickSection(pick),
		Crumb: "Install",
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
// copy (swapping the url and flagging archived so pinInstall keeps the canonical
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

// cloneModeOptions renders the branch-install Package/Clone modes stacked
// vertically, the active one marked and highlighted (mirrors installSourceOptions).
func cloneModeOptions(mode int) string {
	active := lipgloss.NewStyle().Foreground(core.FocusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(core.MutedColor)
	opts := []struct{ label, desc string }{
		{"Package", "download the branch and install as an unzipped package"},
		{"Clone", "git clone the repo as a live working copy (keeps .git)"},
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

// cloneModeWarning cautions that clone mode places the whole repo at the addon
// path, so it only works for repos whose root is the addon itself.
func cloneModeWarning(selected addon.Addon) string {
	dest := selected.Path
	if dest == "" {
		dest = addon.DefaultPath(selected.Name)
	}
	warn := lipgloss.NewStyle().Foreground(core.MutedColor)
	return warn.Render("  ⚠ clones the whole repo (with .git) to " + dest + ";\n    the repo root must be the addon itself or it won't load in Godot.")
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
		Crumb:  "Remove",
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

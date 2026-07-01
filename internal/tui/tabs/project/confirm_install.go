package project

import (
	"fmt"
	"path"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/store"
	"gdaddon/internal/tui/flows/packages"
	"gdaddon/internal/tui/widgets"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// installEndpoint adapts the install confirm into a packages.Endpoint: it captures the
// selected addon and its installed version, and builds the confirm for whichever
// release/branch/asset the shared browse flow hands back.
func installEndpoint(selected addon.Addon, local string) packages.Endpoint {
	return func(sel packages.Selection) core.Screen {
		// Store assets have no git asset/clone/branch variants — confirm the chosen
		// version and install it via the store path (resolved by version at install).
		if store.IsStoreURL(selected.URL) {
			return newStoreInstallConfirm(selected, local, sel.Tag)
		}
		pick := versionItem{tag: sel.Tag, asset: sel.Asset, archivedAsset: sel.ArchivedAsset, repoID: sel.RepoID, branch: sel.Branch, archived: sel.Archived}
		return newInstallConfirm(selected, local, pick)
	}
}

// pinnedInstallScreen reinstalls exactly what the manifest pins for `a` — the same
// per-entry install InstallAll performs — reusing installEndpoint's confirm builders but
// sourcing the version/asset from the manifest entry instead of a browsed selection. A
// clone entry carries pick.clone so newInstallTask git-clones the recorded branch (via
// cloneInstall) rather than unzipping the .git url as a package.
func pinnedInstallScreen(a addon.Addon, local string) core.Screen {
	if store.IsStoreURL(a.URL) {
		return newStoreInstallConfirm(a, local, a.Version) // store pins Version
	}
	repoID, _ := source.RepoID(a.URL)
	pick := versionItem{tag: a.Tag, repoID: repoID, clone: a.IsClone(),
		asset: source.Asset{Name: path.Base(a.URL), URL: a.URL}}
	return newInstallConfirm(a, local, pick) // plain (non-branch/non-archived) confirm → newInstallTask
}

// newStoreInstallConfirm confirms installing a chosen Asset Store version, then runs
// the store install task pinned to that version.
func newStoreInstallConfirm(selected addon.Addon, local, version string) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Render: func(sh *core.Shared) string {
			path := selected.Path
			if path == "" {
				path = "(derived on install)"
			}
			return sh.Box(fmt.Sprintf(
				"Install %s\n\n  version:  %s\n  path:     %s",
				selected.Name, version, path))
		},
		OnYes: core.Replace(newStoreInstallTask(selected, local, version)),
	})
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

// branch install modes (also the vertical option order). Clone is first/default —
// it keeps a real .git; the pinned package snapshot is the de-emphasized second.
const (
	installCloneMode   = iota // git clone the repo as a live working copy (keeps .git)
	installPackageMode        // download + unzip the branch's HEAD commit as a pinned package
)

var installCloneToggleHelp = []key.Binding{
	core.Hint("mode", core.Keys.Up, core.Keys.Down),
	core.Hint("confirm", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

func newInstallConfirm(selected addon.Addon, local string, pick versionItem) *components.DialogScreen {
	// Branch (HEAD) installs offer a Package/Clone mode toggle: clone installs the
	// branch as a live git working copy for development (see cloneModeOptions).
	if pick.branch {
		return widgets.NewToggleConfirm(widgets.ToggleConfirm{
			Crumb: "Install",
			Count: 2,
			Start: installCloneMode,
			Render: func(sh *core.Shared, mode int) string {
				body := confirmInstallBody(sh, selected, pick)
				body += "\n\n  mode:\n" + cloneModeOptions(mode)
				if mode == installCloneMode {
					body += "\n\n" + cloneModeWarning(selected)
				} else {
					body += "\n\n" + packageModeWarning(pick)
				}
				return sh.Box(body)
			},
			OnPick: func(sh *core.Shared, mode int) core.Action {
				p := pick
				p.clone = mode == installCloneMode
				return core.Replace(newInstallTask(selected, local, p))
			},
			Help: installCloneToggleHelp,
		})
	}
	// No archived copy ⇒ the plain confirm (current behavior).
	if pick.archivedAsset.URL == "" {
		return components.CreateConfirmScreen(components.ConfirmSimple{
			Render: func(sh *core.Shared) string { return sh.Box(confirmInstallBody(sh, selected, pick)) },
			OnYes:  core.Replace(newInstallTask(selected, local, pick)),
		})
		// return &components.DialogScreen{
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
	return widgets.NewToggleConfirm(widgets.ToggleConfirm{
		// Title: pickSection(pick),
		Crumb: "Install",
		Count: 2,
		Start: installDownload,
		Render: func(sh *core.Shared, mode int) string {
			body := confirmInstallBody(sh, selected, effectivePick(pick, mode))
			return sh.Box(body + "\n\n  source:\n" + installSourceOptions(mode))
		},
		OnPick: func(sh *core.Shared, mode int) core.Action {
			return core.Replace(newInstallTask(selected, local, effectivePick(pick, mode)))
		},
		Help: installToggleHelp,
	})
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
	return widgets.RenderToggle(mode, []widgets.ToggleOpt{
		{Label: "Download", Desc: "fetch a fresh copy from the remote"},
		{Label: "Archive", Desc: "install from the local archived copy"},
	})
}

// cloneModeOptions renders the branch-install Package/Clone modes stacked
// vertically, the active one marked and highlighted (mirrors installSourceOptions).
func cloneModeOptions(mode int) string {
	return widgets.RenderToggle(mode, []widgets.ToggleOpt{
		{Label: "Clone", Desc: "git clone the repo as a live working copy (keeps .git)"},
		{Label: "Package", Desc: "download & pin the branch's current commit as an unzipped package"},
	})
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

// packageModeWarning cautions that a branch package is a clone without git's
// utility: a static snapshot pinned to the branch's current HEAD commit (no .git,
// no auto-update). Re-installing moves the pin; a submodule fits an arbitrary commit.
func packageModeWarning(pick versionItem) string {
	pin := "HEAD"
	if pick.asset.Commit != "" {
		pin = shortSHA(pick.asset.Commit)
	}
	warn := lipgloss.NewStyle().Foreground(core.MutedColor)
	return warn.Render("  ⚠ a clone without git's utility: installs a snapshot pinned to\n    commit " + pin + " (no .git, no auto-update). Re-install to move the\n    pin; for an arbitrary commit use a submodule.")
}

// shortSHA abbreviates a commit sha for display, leaving shorter refs untouched.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

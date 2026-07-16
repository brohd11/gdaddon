package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/editmanifest"
	"gdaddon/internal/tui/flows/packages"
	"gdaddon/internal/tui/sysopen"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newSubmenuScreen builds the per-addon command submenu (the screen reached by
// pressing enter on an addon row). Install opens the version-fetch flow; Archive
// (offered only when the addon is installed) opens the archive submenu; Remove
// opens the remove confirm. Each row carries its own Pick. A submodule is managed by
// the parent repo, so its install/update-oriented rows (Install, Archive, Export) are
// omitted — only the utility actions (Get deps, Open, Edit Manifest, Remove) remain.
func newSubmenuScreen(st addon.Status, sh *core.Shared) *components.PickerScreen {
	a, local := st.Addon, st.LocalVersion
	c := appctx.Of(sh)
	submodule := a.IsSubmodule()

	var items []list.Item
	// A git checkout on a different branch than the manifest records: offer to re-record
	// the manifest tag to the live branch (the reconcile action; the checkout is source of
	// truth and is never overwritten).
	if st.State == addon.StateBranchChanged {
		items = append(items, components.Item{
			Name: "⎇ Update branch record",
			Desc: "re-record this checkout's current branch (" + st.LiveBranch + ") in the manifest",
			Pick: func(sh *core.Shared) core.Action { return updateBranchRecord(sh, st) },
		})
	}
	// A present checkout (clone or submodule) gets the git command hub: status, fetch, pull,
	// push, commit. A submodule qualifies — it's a real checkout you develop in; only
	// gdaddon's *install* actions are meaningless for one.
	if a.IsGitWorkdir() && st.Present() {
		items = append(items, components.Item{
			Name: "⎇ Git",
			Desc: "status, fetch, pull, push, commit",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newGitSubmenu(st, sh)) },
		})
	}
	if !submodule {
		items = append(items, components.Item{
			Name: "↧ Install / update",
			Desc: "pick a version, branch, or asset to install",
			Pick: func(sh *core.Shared) core.Action {
				// BrowseRepo lists store releases for a store url and git versions
				// otherwise; installEndpoint branches on the same to build the right
				// confirm/task. Gate behind the dirty-checkout confirm since an install
				// overwrites the clone.
				return guardDirty(sh, st, packages.BrowseRepo(a.URL, packages.BrowseOpts{
					Source:         packages.SourceAll,
					IncludeHEAD:    true,
					LeadItems:      append(latestInstallItems(st, c.UpdateChecks[a.Name]), pinnedInstallItems(st)...),
					Endpoint:       installEndpoint(a, local),
					ArchivedMarker: "(archived)",
				}))
			},
		})
	}
	// Lock pins the entry: a later step suppresses its update alerts and makes
	// Install / update reinstall the pinned version. Offered only for package entries
	// with a url (the things that carry a version pin); clones/submodules are live git
	// checkouts with no version to pin.
	if !a.IsGitWorkdir() && a.URL != "" {
		lockName, lockDesc := "🔒 Lock", "pin this version — stop update alerts"
		if a.IsLocked() {
			lockName, lockDesc = "🔓 Unlock", "resume update checks"
		}
		items = append(items, components.Item{
			Name: lockName,
			Desc: lockDesc,
			Pick: func(sh *core.Shared) core.Action { return toggleLock(sh, st) },
		})
	}
	// Offered only for an entry auto-added as another plugin's dependency: clear the
	// is_dependency flag so the user adopts it as their own and it stops flagging as an
	// "unused dependency" once its depender is gone. Mirrors the Lock toggle.
	if a.Dependency {
		items = append(items, components.Item{
			Name: "✓ Keep (not a dependency)",
			Desc: "clear the dependency flag — stop the 'unused dependency' warning",
			Pick: func(sh *core.Shared) core.Action { return keepAddon(sh, st) },
		})
	}
	// Offered whenever the installed addon declares any dependencies (a stable
	// inspection point that stays put once they're resolved), opening the Dependencies
	// screen: per-dep install status, add, and suppress.
	if a.URL != "" && st.Present() && len(c.DepStatuses[a.Name]) > 0 {
		items = append(items, components.Item{
			Name: "⛓ Dependencies",
			Desc: "view this plugin's dependencies and their install status",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newDepsScreen(st, sh)) },
		})
	}
	if !submodule && a.URL != "" && !st.InGlobal(c.GlobalAddons) {
		items = append(items, components.Item{
			Name: "⬆ Export to Global",
			Desc: "add this plugin to your global library (~/.gdaddon)",
			Pick: func(sh *core.Shared) core.Action { return exportToGlobal(sh, a) },
		})
	}
	if st.Present() || a.URL != "" {
		items = append(items, components.Item{
			Name: "\u00BB Open",
			Desc: "open the plugin's install path or source url",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newOpenSubmenu(st)) },
		})
	}
	if !submodule {
		items = append(items, components.Item{
			Name: "⛃ Archive",
			Desc: "browse the repo's versions and save a local copy",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newArchiveSubmenu(st, sh)) },
		})
	}
	items = append(items, components.Item{
		Name: "✎ Edit Manifest",
		Desc: "edit this plugin's manifest entry (url, path, version, tag, kind)",
		Pick: func(sh *core.Shared) core.Action {
			return core.Push(editmanifest.New(appctx.Of(sh).ManifestPath, a, appctx.ProjectDirty{}, false))
		},
	})
	items = append(items, components.Item{
		Name: "✗ Remove",
		Desc: "remove from the project (and optionally delete files)",
		Pick: func(sh *core.Shared) core.Action { return guardDirty(sh, st, newRemoveConfirm(st)) },
	})

	return components.NewPicker(items, components.PickerOpts{
		// Crumb:   "Plugin",
		Title:   a.Name,
		PopStop: true, // the per-addon command hub: sub-flows PopTo() back here
	})
}

// guardDirty interposes a "there are uncommitted changes" confirm before target when the
// addon is a present git checkout with a dirty working tree (the cached GitDirty flag) —
// an install overwrites the clone and a remove may delete its files, so we warn first. On
// Yes the confirm replaces itself with target (so a later back from target returns to the
// submenu, not the confirm); a clean checkout / package entry pushes target directly. The
// caller builds target either way (both constructors are pure).
func guardDirty(sh *core.Shared, st addon.Status, target core.Screen) core.Action {
	if !appctx.Of(sh).GitDirty[st.Addon.Name] {
		return core.Push(target)
	}
	return core.Push(components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb: "Uncommitted",
		Text:  "There are uncommitted changes in this repository,\nare you sure you want to continue?",
		OnYes: core.Replace(target),
	}))
}

// pinnedInstallItems returns the "install what the manifest pins" lead row for the install
// versions picker, or nil (no row) otherwise. Returning a slice matches BrowseOpts.LeadItems
// and leaves room for more lead rows later. A package needs a url and a pinned version/tag;
// a clone offers to check out its recorded branch, but only when not yet cloned (a present
// checkout is a live workdir gdaddon never overwrites); a submodule is never installable.
func pinnedInstallItems(st addon.Status) []list.Item {
	a := st.Addon
	if a.URL == "" || a.IsSubmodule() {
		return nil
	}
	name, desc := "", "reinstall the pinned version"
	if a.IsClone() {
		if st.Present() {
			return nil // a live checkout is never overwritten
		}
		branch := a.Tag
		if branch == "" {
			branch = "HEAD"
		}
		name = "⧉ Clone (" + branch + ")"
		desc = "clone the recorded branch"
	} else {
		ver := a.Version
		if ver == "" {
			ver = a.Tag
		}
		if ver == "" {
			return nil // nothing pinned to reinstall — browse instead
		}
		name = "↺ Install pinned (" + ver + ")"
	}
	return []list.Item{components.Item{
		Name: name,
		Desc: desc,
		Pick: func(sh *core.Shared) core.Action { return core.Push(pinnedInstallScreen(a, st.LocalVersion)) },
	}}
}

// latestInstallItems returns the "install the newest release" lead row for the install
// versions picker, shown only when an update check found a newer release than the pinned
// one (UpdateAvailable — which already excludes clones/submodules/locked/commit-pinned).
// On pick it resolves the latest release's asset off-thread and drops to the install
// confirm, so it reuses the normal install task/pin path.
func latestInstallItems(st addon.Status, info addon.UpdateInfo) []list.Item {
	a := st.Addon
	if info.State != addon.UpdateAvailable {
		return nil
	}
	return []list.Item{components.Item{
		Name: "⬆ Install latest (" + info.LatestTag + ")",
		Desc: "update to the newest release",
		Pick: func(sh *core.Shared) core.Action { return core.Push(latestInstallScreen(a, st.LocalVersion)) },
	}}
}

// newOpenSubmenu builds the Open command submenu: reveal the installed plugin
// directory in the OS file manager (only when installed) and/or open the source
// url in the default browser (only when a url is set). Selecting a row fires the
// open asynchronously and leaves the submenu open.
func newOpenSubmenu(st addon.Status) *components.PickerScreen {
	items := []list.Item{}
	if st.Present() {
		items = append(items, components.Item{
			Name: "Path",
			Desc: st.FullPath,
			Pick: func(sh *core.Shared) core.Action { return sysopen.Path(st.FullPath, false) },
		})
		items = append(items, components.Item{
			Name: "Terminal",
			Desc: st.FullPath,
			Pick: func(sh *core.Shared) core.Action { return sysopen.Terminal(st.FullPath) },
		})
	}
	if st.Addon.URL != "" {
		items = append(items, components.Item{
			Name: "Source",
			Desc: st.Addon.URL,
			Pick: func(sh *core.Shared) core.Action { return sysopen.URL(st.Addon.URL) },
		})
	}
	return components.NewPicker(items, components.PickerOpts{
		Crumb:   "Open",
		Title:   st.Addon.Name,
		PopStop: true,
	})
}

// toggleLock flips the manifest entry's lock flag (SetLock writes/removes the
// `lock: true` line), logs it, broadcasts ProjectDirty so the list reloads, and
// re-renders the submenu so its Lock/Unlock row reflects the new state.
func toggleLock(sh *core.Shared, st addon.Status) core.Action {
	newLock, verb, err := appctx.LockToggle(appctx.Of(sh).ManifestPath, st.Addon.Name, st.Addon.Lock)
	if err != nil {
		return core.StatusErr(err)
	}
	st.Addon.Lock = newLock
	return core.Seq(
		core.SetStatus(verb+" "+st.Addon.Name),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.Replace(newSubmenuScreen(st, sh)),
	)
}

// keepAddon clears the entry's is_dependency flag (SetIsDependency removes the line),
// promoting an auto-added dependency to a user-chosen plugin so it no longer flags as an
// "unused dependency". It logs the change, broadcasts ProjectDirty so the list marker
// clears, and re-renders the submenu (without its Keep row) to reflect the new state.
func keepAddon(sh *core.Shared, st addon.Status) core.Action {
	if err := addon.SetIsDependency(appctx.Of(sh).ManifestPath, st.Addon.Name, false); err != nil {
		return core.StatusErr(err)
	}
	st.Addon.Dependency = false
	return core.Seq(
		core.SetStatus("keeping "+st.Addon.Name+" (no longer a dependency)"),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.Replace(newSubmenuScreen(st, sh)),
	)
}

// updateBranchRecord re-records the manifest entry's tag to the checkout's live branch
// (UpdateEntry leaves url/path/version untouched), reconciling detected branch drift. It
// logs the change and broadcasts ProjectDirty so the list reloads and the branch-changed
// marker clears, then pops back to the browse list.
func updateBranchRecord(sh *core.Shared, st addon.Status) core.Action {
	c := appctx.Of(sh)
	if err := addon.UpdateEntry(c.ManifestPath, st.Addon.Name, "", "", "", st.LiveBranch); err != nil {
		return core.StatusErr(err)
	}
	return core.Seq(
		core.SetStatus("recorded branch "+st.LiveBranch+" for "+st.Addon.Name),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.Pop(),
	)
}

// exportToGlobal copies the project addon into the global list, stripping the
// (often release/archive-pinned) url down to its canonical repo url and carrying
// the project-relative path along as the global entry's remembered default. It then broadcasts
// GlobalDirty (Focus false → the Global list reloads silently without leaving the
// Project tab) and pops the submenu back. The row that triggers this is only shown
// when the repo isn't already in the global list (addon.InGlobalList).
func exportToGlobal(sh *core.Shared, a addon.Addon) core.Action {
	url := a.URL
	if stripped, err := source.RepoURL(a.URL); err == nil {
		url = stripped
	}
	globalPath, err := addon.GlobalListPath()
	if err == nil {
		err = addon.AddEntry(globalPath, a.Name, url, a.Path)
	}
	if err != nil {
		return core.SeqErr(err, core.ResetToRoot())
	}
	return core.Seq(
		core.SetStatus("added "+a.Name+" to global list"),
		core.PropagateAll(appctx.GlobalDirty{}),
		core.Pop(),
	)
}

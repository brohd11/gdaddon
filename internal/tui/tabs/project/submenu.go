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
					LeadItems:      pinnedInstallItems(st),
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
	// Offered only when the addon is installed and actually has unsatisfied deps
	// (the cached check), so a fully-satisfied addon doesn't show a no-op action.
	if a.URL != "" && st.Present() && len(c.DepChecks[a.Name]) > 0 {
		items = append(items, components.Item{
			Name: "⛓ Get dependencies",
			Desc: "add this plugin's missing dependencies to the manifest (Install All to install)",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newGetDepsLoading(st, sh)) },
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
	c := appctx.Of(sh)
	newLock := !st.Addon.Lock
	if err := addon.SetLock(c.ManifestPath, st.Addon.Name, newLock); err != nil {
		return core.SetStatusAndLog("error: " + err.Error())
	}
	st.Addon.Lock = newLock
	verb := "locked"
	if !newLock {
		verb = "unlocked"
	}
	return core.Seq(
		core.SetStatus(verb+" "+st.Addon.Name),
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
		return core.SetStatusAndLog("error: " + err.Error())
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
		return core.Seq(
			core.SetStatusAndLog("error: "+err.Error()),
			core.ResetToRoot(),
		)
	}
	return core.Seq(
		core.SetStatus("added "+a.Name+" to global list"),
		core.PropagateAll(appctx.GlobalDirty{}),
		core.Pop(),
	)
}

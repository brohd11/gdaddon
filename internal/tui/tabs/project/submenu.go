package project

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/editmanifest"
	"github.com/brohd11/gdaddon/internal/tui/flows/packages"
	"github.com/brohd11/gdaddon/internal/tui/sysopen"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repoui"

	"charm.land/bubbles/v2/list"
)

// newSubmenuScreen is an addon's command menu (enter on a row): Install, Archive (when
// installed), Remove and more. A submodule gets only the utility rows (Get deps, Open,
// Edit Manifest, Remove).
func newSubmenuScreen(st addon.Status, sh *core.Shared) *components.PickerScreen {
	a, local := st.Addon, st.LocalVersion
	c := appctx.Of(sh)
	submodule := a.IsSubmodule()

	var items []list.Item
	// Offer to re-record the manifest's branch to the live one (the checkout is never
	// overwritten).
	if st.State == addon.StateBranchChanged {
		items = append(items, components.Item{
			Name: "⎇ Update branch record",
			Desc: "re-record this checkout's current branch (" + st.LiveBranch + ") in the manifest",
			Pick: func(sh *core.Shared) core.Action { return updateBranchRecord(sh, st) },
		})
	}
	// A present checkout, submodules included, gets the Git menu.
	if a.IsGitWorkdir() && st.Present() {
		items = append(items, components.Item{
			Name: "⎇ Git",
			Desc: "status, fetch, pull, push, commit",
			Pick: func(sh *core.Shared) core.Action { return core.Push(repoui.RepoMenu(sh, repoFromStatus(st))) },
		})
	}
	if !submodule {
		items = append(items, components.Item{
			Name: "↧ Install / update",
			Desc: "pick a version, branch, or asset to install",
			Pick: func(sh *core.Shared) core.Action {
				// BrowseRepo handles store and git urls alike. Guarded by the dirty-checkout confirm, since
				// an install overwrites a clone.
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
	// Lock is offered for packages with a url; checkouts have no version to pin.
	if !a.IsGitWorkdir() && a.URL != "" {
		items = append(items, appctx.LockItem(a.IsLocked(), func(sh *core.Shared) core.Action { return toggleLock(sh, st) }))
	}
	// Keep clears the is_dependency flag on an auto-added dependency.
	if a.Dependency {
		items = append(items, components.Item{
			Name: "✓ Keep (not a dependency)",
			Desc: "clear the dependency flag — stop the 'unused dependency' warning",
			Pick: func(sh *core.Shared) core.Action { return keepAddon(sh, st) },
		})
	}
	// Dependencies, when the installed addon declares any.
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

	// "t" opens a terminal at a present install path; otherwise it falls through.
	dir := ""
	if st.Present() {
		dir = st.FullPath
	}
	return components.NewPicker(items, components.PickerOpts{
		Title:   a.Label(),
		Dir:     dir,
		PopStop: true, // the per-addon command hub: sub-flows PopTo() back here
	})
}

// guardDirty shows an "uncommitted changes" confirm before target for a dirty checkout
// (installs overwrite, removes may delete), replacing itself with target on yes.
// Otherwise target is pushed directly.
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

// pinnedInstallItems returns the "install what the manifest pins" row: for a package with
// a url and pinned version or tag, or an uncloned clone. Never for submodules.
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

// latestInstallItems returns the "install the newest release" row when an update is
// available; it resolves the asset off the UI thread and opens the install confirm.
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

// newOpenSubmenu reveals the installed folder and/or opens the source url; it stays open.
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
		Title:   st.Addon.Label(),
		PopStop: true,
	})
}

// toggleLock flips the lock, broadcasts ProjectDirty and redraws the submenu.
func toggleLock(sh *core.Shared, st addon.Status) core.Action {
	newLock, verb, err := appctx.LockToggle(appctx.Of(sh).ManifestPath, st.Addon.Name, st.Addon.Lock)
	if err != nil {
		return core.StatusErr(err)
	}
	st.Addon.Lock = newLock
	return core.Seq(
		core.SetStatus(verb+" "+st.Addon.Label()),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.Replace(newSubmenuScreen(st, sh)),
	)
}

// keepAddon clears is_dependency, broadcasts ProjectDirty and redraws the submenu without
// its Keep row.
func keepAddon(sh *core.Shared, st addon.Status) core.Action {
	if err := addon.SetIsDependency(appctx.Of(sh).ManifestPath, st.Addon.Name, false); err != nil {
		return core.StatusErr(err)
	}
	st.Addon.Dependency = false
	return core.Seq(
		core.SetStatus("keeping "+st.Addon.Label()+" (no longer a dependency)"),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.Replace(newSubmenuScreen(st, sh)),
	)
}

// updateBranchRecord re-records the tag to the live branch, broadcasts ProjectDirty and
// pops back.
func updateBranchRecord(sh *core.Shared, st addon.Status) core.Action {
	c := appctx.Of(sh)
	if err := addon.UpdateEntry(c.ManifestPath, st.Addon.Name, "", "", "", st.LiveBranch); err != nil {
		return core.StatusErr(err)
	}
	return core.Seq(
		core.SetStatus("recorded branch "+st.LiveBranch+" for "+st.Addon.Label()),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.Pop(),
	)
}

// exportToGlobal copies the addon to the global list with its canonical repo url and path,
// broadcasts GlobalDirty (the Global list reloads without switching tabs) and pops back.
func exportToGlobal(sh *core.Shared, a addon.Addon) core.Action {
	url := a.URL
	if stripped, err := source.RepoURL(a.URL); err == nil {
		url = stripped
	}
	globalPath, err := addon.GlobalListPath()
	if err == nil {
		// AddEntryFull, not AddEntry: the entry's own name rides along to the global
		// list rather than waiting for some later install there to rediscover it.
		export := a
		export.URL, export.Version, export.Tag, export.Dependency = url, "", "", false
		err = addon.AddEntryFull(globalPath, export)
	}
	if err != nil {
		return core.SeqErr(err, core.ResetToRoot())
	}
	return core.Seq(
		core.SetStatus("added "+a.Label()+" to global list"),
		core.PropagateAll(appctx.GlobalDirty{}),
		core.Pop(),
	)
}

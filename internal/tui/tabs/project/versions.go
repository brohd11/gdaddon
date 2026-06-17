package project

import (
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// newVersionsScreen builds the top-level version picker: HEAD plus one row per
// release. Each row is a self-dispatching components.Item, so there's no bespoke
// Update — HEAD opens the branch fetch, a single-asset release goes straight to
// confirm, a multi-asset release opens the asset picker.
func newVersionsScreen(selected addon.Addon, local string, listing *source.Listing) *components.PickerScreen {
	items := []list.Item{
		components.Item{
			Name: "HEAD",
			Desc: "track a branch (refs/heads)",
			Pick: func(sh *core.Shared) tea.Cmd { return core.Push(newBranchesLoading(selected, local)) },
		},
	}
	for _, r := range listing.Releases {
		r := r
		items = append(items, components.Item{
			Name: r.Tag,
			Desc: releaseDesc(r),
			Pick: func(sh *core.Shared) tea.Cmd {
				if len(r.Assets) == 1 {
					a := r.Assets[0]
					pick := versionItem{tag: r.Tag, asset: a, prerelease: r.Prerelease, archived: isArchived(a)}
					return core.Push(newInstallConfirm(selected, local, pick))
				}
				return core.Push(newAssetPicker(selected, local, r))
			},
		})
	}
	return components.NewPicker(items, components.PickerOpts{Title: core.HeaderTitle(selected.Name, local, "Versions")})
}

func releaseDesc(r source.Release) string {
	d := fmt.Sprintf("%d asset(s)", len(r.Assets))
	if r.Prerelease {
		d += " · prerelease"
	}
	return d
}

// newAssetPicker lists a multi-asset release's assets; each opens the install
// confirm for that asset.
func newAssetPicker(selected addon.Addon, local string, r source.Release) *components.PickerScreen {
	desc := r.Tag
	if r.Prerelease {
		desc += " · prerelease"
	}
	items := make([]list.Item, 0, len(r.Assets))
	for _, a := range r.Assets {
		pick := versionItem{tag: r.Tag, asset: a, prerelease: r.Prerelease, archived: isArchived(a)}
		items = append(items, components.Item{
			Name: a.Name,
			Desc: desc,
			Pick: func(sh *core.Shared) tea.Cmd { return core.Push(newInstallConfirm(selected, local, pick)) },
		})
	}
	return components.NewPicker(items, components.PickerOpts{Title: core.HeaderTitle(selected.Name, local, "Assets "+r.Tag)})
}

// newBranchPicker lists refs/heads; each opens the install confirm for that branch.
func newBranchPicker(selected addon.Addon, local string, branches []source.Asset) *components.PickerScreen {
	items := make([]list.Item, 0, len(branches))
	for _, b := range branches {
		pick := versionItem{tag: b.Name, asset: b, branch: true, archived: isArchived(b)}
		items = append(items, components.Item{
			Name: "branch: " + b.Name,
			Desc: "latest commit · " + b.Name,
			Pick: func(sh *core.Shared) tea.Cmd { return core.Push(newInstallConfirm(selected, local, pick)) },
		})
	}
	return components.NewPicker(items, components.PickerOpts{Title: core.HeaderTitle(selected.Name, local, "Branches")})
}

// newReleasesLoading builds the loading screen for an addon's release fetch. Its
// onResult folds in archived packages and opens the versions screen (or pops on a
// hard failure with no archive fallback) — the merge/next-screen logic the generic
// loadingScreen no longer owns.
func newReleasesLoading(a addon.Addon, local string) *components.LoadingScreen {
	onResult := func(sh *core.Shared, msg tea.Msg) tea.Cmd {
		m, ok := msg.(releasesMsg)
		if !ok {
			return nil
		}
		var archived []source.Release
		if repoID, err := source.RepoID(a.URL); err == nil {
			archived, _ = archive.List(repoID)
		}
		if m.err != nil && len(archived) == 0 {
			sh.StatusMsg = "error: " + m.err.Error()
			return core.Pop()
		}
		listing := archive.Merge(cloneListing(m.listing), archived)
		return core.Replace(newVersionsScreen(a, local, listing))
	}
	return components.NewLoadingScreen(core.HeaderTitle(a.Name, local, ""), "fetching versions…", fetchReleases(a.URL), onResult)
}

// newBranchesLoading builds the loading screen for a HEAD/branch fetch. Its onResult
// opens the branch picker (or unwinds on error / empty).
func newBranchesLoading(a addon.Addon, local string) *components.LoadingScreen {
	onResult := func(sh *core.Shared, msg tea.Msg) tea.Cmd {
		m, ok := msg.(branchesMsg)
		if !ok {
			return nil
		}
		if m.err != nil {
			sh.StatusMsg = "error: " + m.err.Error()
			return core.ResetToRoot()
		}
		if len(m.branches) == 0 {
			sh.StatusMsg = "no branches found"
			return core.Pop()
		}
		return core.Replace(newBranchPicker(a, local, m.branches))
	}
	return components.NewLoadingScreen(core.HeaderTitle(a.Name, local, ""), "fetching branches…", fetchBranches(a.URL), onResult)
}

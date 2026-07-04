package packages

import (
	"fmt"

	arch "gdaddon/internal/archive"
	"gdaddon/internal/source"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// newVersionsPicker lists a repo's versions (newest first). When opts.IncludeHEAD a
// HEAD row is prepended (lazily fetches branches). A version with a single asset drops
// straight to its endpoint menu; multiple assets open an asset picker first (mirrors
// the project versions.go release rule).
// A release with a local copy is tagged opts.marker(): for marking/install flows that's
// a release whose assets are all in `archived`, plus (SourceAll only) archive-only rows
// whose assets are themselves local. A SourceAll release with a local twin also carries
// it on the Selection (releaseSelection) so the install confirm can offer a source toggle.
func newVersionsPicker(repoID, repoURL string, opts BrowseOpts, releases []source.Release, archived archivedSet) *components.PickerScreen {
	var items []list.Item
	items = append(items, opts.LeadItems...)
	if opts.IncludeHEAD {
		items = append(items, components.Item{
			Name: "HEAD",
			Desc: "track a branch (refs/heads)",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newBranchesLoading(repoID, repoURL, opts)) },
		})
	}
	for _, rel := range releases {
		rel := rel
		desc := fmt.Sprintf("%d asset(s)", len(rel.Assets))
		if len(rel.Assets) == 1 {
			desc = "1 asset - " + rel.Assets[0].Name
			// desc = stripSuffix(desc) // not sure about this
		}
		if rel.Prerelease {
			desc += " · prerelease"
		}
		// A commit-pinned branch package (archived as <branch>@<sha>) surfaces its sha.
		if len(rel.Assets) > 0 && rel.Assets[0].Commit != "" {
			desc += " · " + rel.Assets[0].Commit[:min(7, len(rel.Assets[0].Commit))]
		}
		if archived.releaseArchived(rel) || (opts.Source == SourceAll && allLocal(rel)) {
			desc += " · " + opts.marker()
		}
		items = append(items, components.Item{
			Name: rel.Tag,
			Desc: desc,
			Pick: func(sh *core.Shared) core.Action {
				if len(rel.Assets) == 1 {
					return core.Push(opts.Endpoint(releaseSelection(repoID, rel, rel.Assets[0], archived)))
				}
				return core.Push(newAssetPicker(repoID, rel, opts, archived))
			},
		})
	}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Repo", Title: repoID})
}

// NewVersionsPicker lists an archived repo's versions; a thin wrapper over
// newVersionsPicker kept for the Archive tab, which already holds a RepoArchive (no
// HEAD — the local archive has no fetchable branches; nothing to mark).
func NewVersionsPicker(repo arch.RepoArchive, opts BrowseOpts) *components.PickerScreen {
	opts.IncludeHEAD = false
	return newVersionsPicker(repo.ID, "", opts, repo.Releases, nil)
}

// newAssetPicker lists the assets of a multi-asset release; selecting one opens its
// endpoint menu. An asset with a local copy is tagged opts.marker() — either a remote
// asset present in `archived`, or (SourceAll only) an asset that is itself local.
func newAssetPicker(repoID string, rel source.Release, opts BrowseOpts, archived archivedSet) *components.PickerScreen {
	items := make([]list.Item, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		a := a
		name := a.Name
		if archived.has(rel.Tag, a.Name) || (opts.Source == SourceAll && isArchived(a)) {
			name += " " + opts.marker()
		}
		items = append(items, components.Item{
			Name: name,
			Pick: func(sh *core.Shared) core.Action {
				return core.Push(opts.Endpoint(releaseSelection(repoID, rel, a, archived)))
			},
		})
	}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Assets", Title: repoID})
}

// allLocal reports whether every asset of a release is a local archived copy (no
// remote download) — true for archive-only / SourceArchive rows.
func allLocal(rel source.Release) bool {
	if len(rel.Assets) == 0 {
		return false
	}
	for _, a := range rel.Assets {
		if !isArchived(a) {
			return false
		}
	}
	return true
}

// releaseSelection builds the Selection for a chosen release asset. When the chosen
// asset is remote and a local archived copy of it exists, ArchivedAsset carries that
// copy so an install confirm can offer a Download/Archive source toggle.
func releaseSelection(repoID string, rel source.Release, a source.Asset, archived archivedSet) Selection {
	sel := Selection{
		RepoID:     repoID,
		Tag:        rel.Tag,
		Asset:      a,
		Prerelease: rel.Prerelease,
		Archived:   isArchived(a),
	}
	if !sel.Archived {
		if ar, ok := archived.get(rel.Tag, a.Name); ok {
			sel.ArchivedAsset = ar
		}
	}
	return sel
}

// newBranchPicker lists refs/heads; each opens its endpoint menu. The branch asset is a
// remote HEAD-archive zip, so the archive endpoint stores it under repoID/<branch>.
func newBranchPicker(repoID string, branches []source.Asset, opts BrowseOpts) *components.PickerScreen {
	items := make([]list.Item, 0, len(branches))
	for _, b := range branches {
		b := b
		desc := "latest commit · " + b.Name
		if b.Commit != "" {
			desc = "pinned · " + b.Commit[:min(7, len(b.Commit))]
		}
		items = append(items, components.Item{
			Name: "branch: " + b.Name,
			Desc: desc,
			Pick: func(sh *core.Shared) core.Action {
				return core.Push(opts.Endpoint(Selection{RepoID: repoID, Tag: b.Name, Asset: b, Branch: true}))
			},
		})
	}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Branches", Title: repoID})
}

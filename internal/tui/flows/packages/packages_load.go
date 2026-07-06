package packages

import (
	"context"

	arch "gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/store"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// BrowseRepo is the single-repo entry point: it lists one repo's versions, sourced
// per opts.Source, and runs opts.Endpoint on the chosen package. SourceArchive builds
// synchronously from the local archive (no HEAD — nothing fetchable); SourceRemote and
// SourceAll fetch upstream first (SourceAll also folds in any archived versions), so
// they return a loading screen that resolves into the picker.
func BrowseRepo(repoURL string, opts BrowseOpts) core.Screen {
	repoID, _ := source.RepoID(repoURL)
	if store.IsStoreURL(repoURL) {
		opts.IncludeHEAD = false // store assets have no branches → no HEAD/"git clone"
	}
	if opts.Source == SourceArchive {
		opts.IncludeHEAD = false // local archive has no fetchable branches
		releases, _ := arch.List(repoID)
		return newVersionsPicker(repoID, "", opts, releases, nil)
	}
	return newReleasesLoading(repoID, repoURL, opts)
}

// newReleasesLoading fetches a repo's upstream versions, consults the local archive
// (when SourceAll or MarkArchived), then replaces itself with the versions picker.
//
// Both SourceAll (install) and MarkArchived (archive) keep one row per remote version
// and tag the ones with a local copy via opts.marker(). MarkArchived stops there (you
// can't archive a non-remote version); SourceAll additionally lists archive-only
// versions (delisted upstream / archived branch HEAD) as their own rows installed from
// the local copy. On a hard fetch failure it pops with a status — except a SourceAll
// browse can still fall back to an archive-only listing.
func newReleasesLoading(repoID, repoURL string, opts BrowseOpts) *components.LoadingScreen {
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		m, ok := msg.(releasesMsg)
		if !ok {
			return core.Action{}
		}
		var archived []source.Release
		if opts.Source == SourceAll || opts.MarkArchived {
			archived, _ = arch.List(repoID)
		}
		set := buildArchivedSet(archived)

		if opts.MarkArchived {
			if m.err != nil { // no remote ⇒ nothing new to archive
				return core.SeqErr(m.err, core.Pop())
			}
			return core.Replace(newVersionsPicker(repoID, repoURL, opts, cloneListing(m.listing).Releases, set))
		}

		if m.err != nil {
			if len(archived) == 0 {
				return core.SeqErr(m.err, core.Pop())
			}
			// offline / delisted: install straight from the archive-only listing.
			return core.Replace(newVersionsPicker(repoID, repoURL, opts, archived, set))
		}
		releases := cloneListing(m.listing).Releases
		releases = append(releases, archiveOnly(releases, archived)...)
		return core.Replace(newVersionsPicker(repoID, repoURL, opts, releases, set))
	}
	return components.NewLoadingScreen(repoID, "fetching versions…", fetchReleases(repoURL), onResult)
}

// archiveOnly returns the archived releases whose tag is absent from the remote
// releases, so an install browse still surfaces versions no longer upstream (their
// assets are local copies, installed without a download).
func archiveOnly(remote, archived []source.Release) []source.Release {
	have := make(map[string]bool, len(remote))
	for _, r := range remote {
		have[r.Tag] = true
	}
	var out []source.Release
	for _, ar := range archived {
		if !have[ar.Tag] {
			out = append(out, ar)
		}
	}
	return out
}

func fetchReleases(url string) func(context.Context) tea.Cmd {
	return func(ctx context.Context) tea.Cmd {
		return func() tea.Msg {
			if store.IsStoreURL(url) {
				listing, err := store.Listing(ctx, url)
				return releasesMsg{listing: listing, err: err}
			}
			listing, err := source.AvailableVersions(ctx, url)
			return releasesMsg{listing: listing, err: err}
		}
	}
}

func fetchBranches(url string) func(context.Context) tea.Cmd {
	return func(ctx context.Context) tea.Cmd {
		return func() tea.Msg {
			branches, err := source.Branches(ctx, url)
			return branchesMsg{branches: branches, err: err}
		}
	}
}

// cloneListing copies a listing's release/asset slices so merging archived assets in
// doesn't mutate the cached upstream listing. A nil listing clones to nil.
func cloneListing(l *source.Listing) *source.Listing {
	if l == nil {
		return nil
	}
	c := *l
	c.Releases = make([]source.Release, len(l.Releases))
	for i, r := range l.Releases {
		r.Assets = append([]source.Asset(nil), r.Assets...)
		c.Releases[i] = r
	}
	return &c
}

// newBranchesLoading fetches the repo's branches as HEAD-archive assets, then opens the
// branch picker (or unwinds on error / empty).
func newBranchesLoading(repoID, repoURL string, opts BrowseOpts) *components.LoadingScreen {
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		m, ok := msg.(branchesMsg)
		if !ok {
			return core.Action{}
		}
		if m.err != nil {
			return core.SeqErr(m.err, core.Pop())
		}
		if len(m.branches) == 0 {
			return core.Seq(
				core.SetStatusAndLog("no branches found"),
				core.Pop(),
			)
		}
		return core.Replace(newBranchPicker(repoID, m.branches, opts))
	}
	return components.NewLoadingScreen(repoID, "fetching branches...", fetchBranches(repoURL), onResult)
}

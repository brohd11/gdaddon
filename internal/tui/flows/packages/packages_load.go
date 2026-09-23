package packages

import (
	"context"

	arch "github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/store"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "charm.land/bubbletea/v2"
)

// BrowseRepo lists one repo's versions from opts.Source and runs opts.Endpoint on the
// choice. SourceArchive builds from the local archive synchronously; SourceRemote and
// SourceAll (which also folds in archived versions) fetch first behind a loading screen.
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

// newReleasesLoading fetches upstream versions, checks the archive (SourceAll or
// MarkArchived), and opens the versions picker. Both mark remote versions with a local
// copy; SourceAll also lists archive-only versions. On a fetch failure it pops, except
// SourceAll, which can fall back to the archive alone.
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

// archiveOnly returns archived releases missing upstream, so installs can still offer
// them from the local copy.
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

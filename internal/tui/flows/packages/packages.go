// Package packages is a shared, domain-aware browsing flow: the navigation chain
// repo → versions → asset → per-package action, reused by more than one tab. Each
// caller parameterizes it with a BrowseOpts (where versions are drawn from, whether
// to offer branch HEADs, and the per-package command menu), so the navigation is
// shared while the leaf action differs (Archive tab → Remove, Global → Add to archive).
package packages

import (
	"context"
	"fmt"
	"strings"

	arch "gdaddon/internal/archive"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// Display tokens for archived packages, kept here for easy editing.
const (
	archivedSuffix = " - archived"        // suffix on an asset sourced from the local archive
	archivedMarker = "(already archived)" // marks a remote version that already has a local copy
)

// Source selects where a package flow draws its versions from.
type Source int

const (
	SourceArchive Source = iota // local archive only (synchronous)
	SourceRemote                // upstream releases only (async fetch)
	SourceAll                   // upstream releases + archived, merged
)

// Endpoint builds the per-package command menu for the chosen asset. tag is the
// release tag (or branch name) the asset came from (the archive action needs it to
// store under repo/tag; a remove can ignore it).
type Endpoint func(repoID, tag string, asset source.Asset) *components.PickerScreen

// BrowseOpts configures a browse flow. It is threaded through the flow's screens
// (rather than unpacked into positional args) so new knobs can be added without
// touching every signature.
type BrowseOpts struct {
	Source       Source   // where versions come from
	IncludeHEAD  bool     // also offer a HEAD row (branch tracking); ignored for archive
	Endpoint     Endpoint // the per-package command menu
	MarkArchived bool     // mark already-archived remote versions instead of listing the local copies (archive flows)
}

// archivedSet indexes a repo's archived packages by tag → stored filename, so a remote
// listing can mark versions that already have a local copy.
type archivedSet map[string]map[string]bool

// buildArchivedSet folds arch.List output (assets named "<file> - archived") into the
// index. Returns nil when there is nothing archived (so callers can treat nil as "no
// annotation").
func buildArchivedSet(archived []source.Release) archivedSet {
	if len(archived) == 0 {
		return nil
	}
	s := make(archivedSet, len(archived))
	for _, rel := range archived {
		names := make(map[string]bool, len(rel.Assets))
		for _, a := range rel.Assets {
			names[strings.TrimSuffix(a.Name, archivedSuffix)] = true
		}
		s[rel.Tag] = names
	}
	return s
}

// has reports whether a remote asset (by tag + name) already has a local copy.
func (s archivedSet) has(tag, assetName string) bool {
	names, ok := s[tag]
	return ok && names[assetName]
}

// releaseArchived reports whether every asset of a remote release is already archived.
func (s archivedSet) releaseArchived(rel source.Release) bool {
	if len(rel.Assets) == 0 {
		return false
	}
	for _, a := range rel.Assets {
		if !s.has(rel.Tag, a.Name) {
			return false
		}
	}
	return true
}

// releasesMsg / branchesMsg carry the result of an upstream fetch back to the loading
// screen's onResult closure.
type releasesMsg struct {
	listing *source.Listing
	err     error
}

type branchesMsg struct {
	branches []source.Asset
	err      error
}

// ---------- repos-list entry ----------

// ReposScreen browses every archived repo (one row per repo); selecting one opens
// its versions. It is archive-scoped — remote hosts have no enumerable repo list —
// so its rows always come from the local archive.
type ReposScreen struct {
	list list.Model
	opts BrowseOpts
}

var _ core.Filterer = (*ReposScreen)(nil)
var _ core.Receiver = (*ReposScreen)(nil)

// BrowseRepos is the repos-list entry point: an archive-wide browser whose chosen
// package runs opts.Endpoint. Source/IncludeHEAD are irrelevant here (the list is the
// local archive) and ignored.
func BrowseRepos(opts BrowseOpts) *ReposScreen {
	return &ReposScreen{
		list: core.NewSelectList(RepoItems(opts), "Archived Packages"),
		opts: opts,
	}
}

// RepoItems reads every archived repo as self-dispatching rows (each opens that
// repo's versions picker); an empty/missing archive yields one inert hint row.
// Exported so the Archive tab root builds its rows from the same source.
func RepoItems(opts BrowseOpts) []list.Item {
	repos, _ := arch.Repos()
	var items []list.Item
	for _, repo := range repos {
		repo := repo // capture per row
		items = append(items, components.Item{
			Name: repo.ID,
			Desc: fmt.Sprintf("%d version(s)", len(repo.Releases)),
			Pick: func(sh *core.Shared) core.Action { return core.Push(NewVersionsPicker(repo, opts)) },
		})
	}
	if len(items) == 0 {
		items = append(items, components.Item{
			Name: "(nothing archived yet)",
			Desc: "archive a package via Project → an addon → Archive",
		})
	}
	return items
}

func (s *ReposScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *ReposScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *ReposScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	return s, components.RootUpdate(sh, &s.list, msg)
}

func (s *ReposScreen) View(*core.Shared) string { return s.list.View() }
func (s *ReposScreen) HelpView(*core.Shared) string {
	return core.ShortHelp(s.list, core.HelpTabbed)
}

// Receive rebuilds the list from disk on an ArchiveDirty broadcast (after a package
// removal), so the screen reflects the change.
func (s *ReposScreen) Receive(sh *core.Shared, payload any) core.Action {
	if _, ok := payload.(appctx.ArchiveDirty); ok {
		s.list.SetItems(RepoItems(s.opts))
	}
	return core.Action{}
}

func (s *ReposScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}

// ---------- single-repo entry ----------

// BrowseRepo is the single-repo entry point: it lists one repo's versions, sourced
// per opts.Source, and runs opts.Endpoint on the chosen package. SourceArchive builds
// synchronously from the local archive (no HEAD — nothing fetchable); SourceRemote and
// SourceAll fetch upstream first (SourceAll also folds in any archived versions), so
// they return a loading screen that resolves into the picker.
func BrowseRepo(repoURL string, opts BrowseOpts) core.Screen {
	repoID, _ := source.RepoID(repoURL)
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
// MarkArchived keeps the remote rows and marks the ones already archived (archive
// flows). Otherwise SourceAll folds the local copies in as selectable "- archived"
// assets (install-style). On a hard fetch failure it pops with a status — except a
// plain SourceAll merge can still fall back to an archive-only listing.
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

		if opts.MarkArchived {
			if m.err != nil { // no remote ⇒ nothing new to archive
				return core.Seq(
					core.SetStatusAndLog("error: "+m.err.Error()),
					core.Pop(),
				)
			}
			listing := cloneListing(m.listing)
			return core.Replace(newVersionsPicker(repoID, repoURL, opts, listing.Releases, buildArchivedSet(archived)))
		}

		if m.err != nil && len(archived) == 0 {
			return core.Seq(
				core.SetStatusAndLog("error: "+m.err.Error()),
				core.Pop(),
			)
		}
		listing := arch.Merge(cloneListing(m.listing), archived)
		return core.Replace(newVersionsPicker(repoID, repoURL, opts, listing.Releases, nil))
	}
	return components.NewLoadingScreen(repoID, "fetching versions…", fetchReleases(repoURL), onResult)
}

func fetchReleases(url string) tea.Cmd {
	return func() tea.Msg {
		listing, err := source.AvailableVersions(context.Background(), url)
		return releasesMsg{listing: listing, err: err}
	}
}

func fetchBranches(url string) tea.Cmd {
	return func() tea.Msg {
		branches, err := source.Branches(context.Background(), url)
		return branchesMsg{branches: branches, err: err}
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

// ---------- versions / asset / branch pickers ----------

// newVersionsPicker lists a repo's versions (newest first). When opts.IncludeHEAD a
// HEAD row is prepended (lazily fetches branches). A version with a single asset drops
// straight to its endpoint menu; multiple assets open an asset picker first (mirrors
// the project versions.go release rule).
// When archived is non-nil (an archive flow), a release all of whose assets already
// have a local copy is marked with archivedMarker.
func newVersionsPicker(repoID, repoURL string, opts BrowseOpts, releases []source.Release, archived archivedSet) *components.PickerScreen {
	var items []list.Item
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
		if archived.releaseArchived(rel) {
			desc += " · " + archivedMarker
		}
		items = append(items, components.Item{
			Name: rel.Tag,
			Desc: desc,
			Pick: func(sh *core.Shared) core.Action {
				if len(rel.Assets) == 1 {
					return core.Push(opts.Endpoint(repoID, rel.Tag, rel.Assets[0]))
				}
				return core.Push(newAssetPicker(repoID, rel, opts, archived))
			},
		})
	}
	return components.NewPicker(items, components.PickerOpts{Title: repoID})
}

// NewVersionsPicker lists an archived repo's versions; a thin wrapper over
// newVersionsPicker kept for the Archive tab, which already holds a RepoArchive (no
// HEAD — the local archive has no fetchable branches; nothing to mark).
func NewVersionsPicker(repo arch.RepoArchive, opts BrowseOpts) *components.PickerScreen {
	opts.IncludeHEAD = false
	return newVersionsPicker(repo.ID, "", opts, repo.Releases, nil)
}

// newAssetPicker lists the assets of a multi-asset release; selecting one opens its
// endpoint menu. When archived is non-nil (an archive flow), assets that already have a
// local copy are marked with archivedMarker.
func newAssetPicker(repoID string, rel source.Release, opts BrowseOpts, archived archivedSet) *components.PickerScreen {
	items := make([]list.Item, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		a := a
		name := a.Name
		if archived.has(rel.Tag, a.Name) {
			name += " " + archivedMarker
		}
		items = append(items, components.Item{
			Name: name,
			Pick: func(sh *core.Shared) core.Action { return core.Push(opts.Endpoint(repoID, rel.Tag, a)) },
		})
	}
	return components.NewPicker(items, components.PickerOpts{Title: repoID + " — " + rel.Tag})
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
			return core.Seq(
				core.SetStatusAndLog("error: "+m.err.Error()),
				core.Pop(),
			)
		}
		if len(m.branches) == 0 {
			return core.Seq(
				core.SetStatusAndLog("no branches found"),
				core.Pop(),
			)
		}
		return core.Replace(newBranchPicker(repoID, m.branches, opts))
	}
	return components.NewLoadingScreen(repoID, "fetching branches…", fetchBranches(repoURL), onResult)
}

// newBranchPicker lists refs/heads; each opens its endpoint menu. The branch asset is a
// remote HEAD-archive zip, so the archive endpoint stores it under repoID/<branch>.
func newBranchPicker(repoID string, branches []source.Asset, opts BrowseOpts) *components.PickerScreen {
	items := make([]list.Item, 0, len(branches))
	for _, b := range branches {
		b := b
		items = append(items, components.Item{
			Name: "branch: " + b.Name,
			Desc: "latest commit · " + b.Name,
			Pick: func(sh *core.Shared) core.Action { return core.Push(opts.Endpoint(repoID, b.Name, b)) },
		})
	}
	return components.NewPicker(items, components.PickerOpts{Title: repoID + " — Branches"})
}

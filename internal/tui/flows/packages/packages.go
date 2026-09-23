// Package packages is the shared browse flow repo → versions → asset → action.
// BrowseOpts sets the version source, HEAD rows and the leaf action (Archive tab: Remove;
// Global: Add to archive).
package packages

import (
	"fmt"
	"strings"

	arch "github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// Display tokens for archived packages, kept here for easy editing.
const (
	archivedSuffix = arch.ArchivedSuffix   // suffix on an asset sourced from the local archive
	archivedMarker = " (already archived)" // marks a remote version that already has a local copy
)

// Source selects where a package flow draws its versions from.
type Source int

const (
	SourceArchive Source = iota // local archive only (synchronous)
	SourceRemote                // upstream releases only (async fetch)
	SourceAll                   // upstream releases + archived, merged
)

// Selection is the chosen package: repo id, tag (release or branch), asset and flags,
// enough for any leaf action.
type Selection struct {
	RepoID        string
	Tag           string
	Asset         source.Asset
	ArchivedAsset source.Asset // a local archived copy of this same remote version, if one exists (install toggle); zero = none
	Branch        bool         // chosen via the HEAD/branches path (vs a release)
	Prerelease    bool         // provider flag or semantic prerelease tag (false for branches)
	Archived      bool         // the asset is a local archived copy (local-file URL)
}

// Endpoint builds the screen pushed for the chosen package (a submenu or a confirm).
type Endpoint func(Selection) core.Screen

// BrowseOpts configures a browse flow, threaded through its screens.
type BrowseOpts struct {
	Source         Source   // where versions come from
	IncludeHEAD    bool     // also offer a HEAD row (branch tracking); ignored for archive
	Endpoint       Endpoint // the per-package command menu
	MarkArchived   bool     // mark already-archived remote versions instead of listing the local copies (archive flows)
	ArchivedMarker string   // override the text tagging an archived version (defaults to archivedMarker)
	// LeadItems are rows above the HEAD row (the project install's "reinstall pinned").
	LeadItems []list.Item
}

// marker returns the text used to tag a version that has a local archived copy.
func (o BrowseOpts) marker() string {
	if o.ArchivedMarker != "" {
		return o.ArchivedMarker
	}
	return archivedMarker
}

// archivedSet indexes archived packages by tag and remote asset name (the " (archived)"
// suffix trimmed), so remote listings can mark and install local copies.
type archivedSet map[string]map[string]source.Asset

// buildArchivedSet indexes arch.List output; nil when nothing is archived.
func buildArchivedSet(archived []source.Release) archivedSet {
	if len(archived) == 0 {
		return nil
	}
	s := make(archivedSet, len(archived))
	for _, rel := range archived {
		assets := make(map[string]source.Asset, len(rel.Assets))
		for _, a := range rel.Assets {
			assets[strings.TrimSuffix(a.Name, archivedSuffix)] = a
		}
		s[rel.Tag] = assets
	}
	return s
}

// has reports whether a remote asset (by tag + name) already has a local copy.
func (s archivedSet) has(tag, assetName string) bool {
	_, ok := s.get(tag, assetName)
	return ok
}

// get returns the local archived asset for a remote asset (by tag + name), if any.
func (s archivedSet) get(tag, assetName string) (source.Asset, bool) {
	assets, ok := s[tag]
	if !ok {
		return source.Asset{}, false
	}
	a, ok := assets[assetName]
	return a, ok
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

// ReposScreen browses every archived repo; remote hosts cannot list repos, so it is
// archive-only.
type ReposScreen struct {
	list list.Model
	opts BrowseOpts
}

var _ core.Filterer = (*ReposScreen)(nil)
var _ core.Receiver = (*ReposScreen)(nil)

// RepoItems builds a row per archived repo (opening its versions), or one hint row.
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
	items = components.EnsurePlaceholder(items, "(nothing archived yet)", "archive a package via Project → an addon → Archive")
	return items
}

func (s *ReposScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *ReposScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *ReposScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	return s, components.RootUpdate(sh, &s.list, msg)
}

func (s *ReposScreen) View(*core.Shared) string { return core.RenderList(s.list) }
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

// The single-repo browse entry (BrowseRepo) and its loading/fetch helpers live in
// packages_load.go; the versions/asset/branch picker builders live in
// packages_pickers.go.

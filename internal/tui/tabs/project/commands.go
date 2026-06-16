package project

import (
	"context"
	"gdaddon/internal/tui/core"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------- fetch commands ----------

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

// cloneListing copies a listing's release/asset slices so merging archived assets
// in doesn't mutate the cached upstream listing. A nil listing clones to nil.
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

// finishInstallCmd pins the freshly installed url, resolved path, and version
// into the manifest and re-inspects, returning msgRootRefresh for the router to
// apply to the browse list.
func finishInstallCmd(sh *core.Shared, selected addon.Addon, pick versionItem, instPath, instVersion string) tea.Cmd {
	manifestPath, projectRoot := sh.ManifestPath, sh.ProjectRoot
	name, url := selected.Name, pick.asset.URL
	// Installing from the local archive must not pin the machine-specific archive
	// path as the manifest url — keep the entry's canonical repo url instead.
	if pick.archived {
		url = ""
	}
	version := instVersion
	if version == "" {
		version = strings.TrimPrefix(pick.tag, "v")
	}

	status := "updated " + name + " → " + version
	return func() tea.Msg {
		_ = addon.UpdateEntry(manifestPath, name, url, instPath, version)
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return core.MsgRootRefresh{Status: status}
		}
		return core.MsgRootRefresh{Status: status, Statuses: statuses}
	}
}

func archiveFinished() tea.Cmd {
	return func() tea.Msg { return core.ArchiveFinishedMsg{} }
}

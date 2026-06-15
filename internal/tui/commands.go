package tui

import (
	"context"
	"fmt"
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

// ---------- streaming task machinery ----------

// startTask spawns run in the background, piping report() lines into the output
// log via the shared events channel, and returns the spinner tick + the wait for
// the first event. run sends the terminating installEvent itself. Shared by
// install, install-all, and archive (see taskScreen).
func startTask(sh *shared, run func(report addon.Reporter, done chan<- installEvent)) tea.Cmd {
	sh.events = make(chan installEvent)
	ch := sh.events
	go func() {
		report := addon.Reporter(func(format string, args ...any) {
			ch <- installEvent{line: fmt.Sprintf(format, args...)}
		})
		run(report, ch)
	}()
	return tea.Batch(sh.spinner.Tick, waitForEvent(ch))
}

func waitForEvent(events chan installEvent) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

// finishInstallCmd pins the freshly installed url, resolved path, and version
// into the manifest and re-inspects, returning installDoneMsg for the router to
// apply to the browse list.
func finishInstallCmd(sh *shared, selected addon.Addon, pick versionItem, instPath, instVersion string) tea.Cmd {
	manifestPath, projectRoot := sh.manifestPath, sh.projectRoot
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

	return func() tea.Msg {
		_ = addon.UpdateEntry(manifestPath, name, url, instPath, version)
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return installDoneMsg{name: name, version: version}
		}
		return installDoneMsg{statuses: statuses, name: name, version: version}
	}
}

// finishInstallAllCmd re-inspects after a batch install for the router to apply.
func finishInstallAllCmd(sh *shared) tea.Cmd {
	manifestPath, projectRoot := sh.manifestPath, sh.projectRoot
	return func() tea.Msg {
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return installAllDoneMsg{}
		}
		return installAllDoneMsg{statuses: statuses}
	}
}

// reloadCmd re-inspects the manifest and returns reloadAddonsMsg so the router
// rebuilds the browse list (after a row was added) and sets the status line.
func reloadCmd(sh *shared, status string) tea.Cmd {
	manifestPath, projectRoot := sh.manifestPath, sh.projectRoot
	return func() tea.Msg {
		statuses, _ := addon.Inspect(manifestPath, projectRoot)
		return reloadAddonsMsg{status: status, statuses: statuses}
	}
}

func archiveFinished() tea.Cmd {
	return func() tea.Msg { return archiveFinishedMsg{} }
}

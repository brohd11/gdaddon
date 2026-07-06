// Package appctx holds gdaddon's domain-specific TUI context: the manifest/project
// paths the tabs operate on, the persistent header that renders them, the tab titles,
// and the Dirty notification payloads the tab roots react to. It is the one place that wires
// the domain to the otherwise agnostic core/components framework — it lives in its
// own leaf package so both the tui package (which imports the tabs) and the tabs
// can read the context without an import cycle.
package appctx

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/selfupdate"

	"github.com/brohd11/bubblestack/core"
	tea "github.com/charmbracelet/bubbletea"
)

// Ctx is the consumer context stored on core.Shared.App. Tabs recover it with Of.
type Ctx struct {
	Version       string // the running binary's version, for the self-update check
	ManifestPath  string
	ProjectRoot   string
	ManifestRel   string // ManifestPath relative to ProjectRoot, for display
	ProjectName   string
	HasProject    bool
	GlobalAddons  []addon.Addon // cached from ~/.gdaddon/plugins.yml
	ArchivedIDs   []string      // cached repo IDs from archive.Repos()
	ProjectAddons []addon.Addon // cached from the project manifest

	// LastSearchQuery is the most recent Search tab query. Session-only (not
	// persisted): it keeps the search form filled across tab navigation but resets
	// on a fresh launch. The Search tab reads it to prefill and writes it on submit.
	LastSearchQuery string

	// UpdateChecks caches the project list's per-addon update-check results,
	// keyed by addon name. It's populated asynchronously (network) by the Project
	// tab and refreshed after a ProjectDirty/PathRefresh; the list reads it back
	// to draw the "update available" marker. A missing key reads as a zero
	// UpdateInfo (UpdateUnknown), i.e. no marker.
	UpdateChecks map[string]addon.UpdateInfo

	// DepChecks caches each project addon's unsatisfied dependencies, keyed by
	// addon name (only addons with >0 missing deps appear). Unlike UpdateChecks it's
	// local-only (compares declared plugin.cfg deps against the manifest), so it's
	// recomputed synchronously in loadProject on every refresh. The list reads it to
	// draw the "missing deps" marker; the per-addon submenu gates its "Get
	// dependencies" item on it.
	DepChecks map[string][]addon.Dependency

	// GitDirty marks each clone entry whose git working tree has uncommitted
	// changes, keyed by addon name (only dirty clones appear). Local-only like
	// DepChecks, recomputed synchronously in loadProject; the Project list reads it
	// to draw the "uncommitted changes" marker.
	GitDirty map[string]bool
}

// New builds the context for a project root and performs the initial path scan.
func New(projectRoot, version string) *Ctx {
	c := &Ctx{ProjectRoot: projectRoot, Version: version}
	c.Scan()
	c.loadGlobal()
	c.loadArchive()
	c.loadProject()
	return c
}

func (c *Ctx) loadGlobal() {
	p, err := addon.GlobalListPath()
	if err != nil {
		c.GlobalAddons = nil
		return
	}
	c.GlobalAddons, _ = addon.Parse(p)
}

func (c *Ctx) loadArchive() {
	repos, _ := archive.Repos()
	ids := make([]string, 0, len(repos))
	for _, r := range repos {
		ids = append(ids, r.ID)
	}
	c.ArchivedIDs = ids
}

func (c *Ctx) loadProject() {
	if c.ManifestPath == "" {
		c.ProjectAddons = nil
		c.DepChecks = nil
		c.GitDirty = nil
		return
	}
	c.ProjectAddons, _ = addon.Parse(c.ManifestPath)
	c.refreshDepChecks()
	c.refreshGitChecks()
}

// refreshGitChecks recomputes which present git-checkout entries (clone or submodule)
// have a dirty working tree. Local-only (a `git status` per checkout), so it rides
// loadProject alongside refreshDepChecks. Reuses Inspect for each entry's resolved
// install path.
func (c *Ctx) refreshGitChecks() {
	checks := make(map[string]bool)
	statuses, _ := addon.Inspect(c.ManifestPath, c.ProjectRoot)
	for _, s := range statuses {
		if s.Addon.IsGitWorkdir() && s.Present() && addon.HasUncommittedChanges(s.FullPath) {
			checks[s.Addon.Name] = true
		}
	}
	c.GitDirty = checks
}

// refreshDepChecks recomputes each addon's unsatisfied dependencies against the
// freshly loaded manifest. Local-only and cheap (reads small plugin.cfg files), so
// it rides every loadProject rather than needing its own async pass.
func (c *Ctx) refreshDepChecks() {
	checks := make(map[string][]addon.Dependency)
	for _, a := range c.ProjectAddons {
		if missing, err := addon.MissingDeps(a, c.ProjectRoot, c.ProjectAddons); err == nil && len(missing) > 0 {
			checks[a.Name] = missing
		}
	}
	c.DepChecks = checks
}

// RefreshAll broadcasts the four Dirty markers so every cached tab root reloads from
// disk. The global Refresh key (Keys.Refresh, wired in tui.Run) and Actions ▸ Refresh
// both return it.
func RefreshAll() core.Action {
	return core.Seq(
		core.PropagateAll(ArchiveDirty{}),
		core.PropagateAll(ProjectDirty{}),
		core.PropagateAll(GlobalDirty{}),
		core.PropagateAll(PathRefresh{}),
		core.SetStatus("Refreshed"),
	)
}

// RefreshGlobal reloads the cached global addon list from disk.
func (c *Ctx) RefreshGlobal() { c.loadGlobal() }

// RefreshArchive reloads the cached archived repo IDs from disk.
func (c *Ctx) RefreshArchive() { c.loadArchive() }

// RefreshProject reloads the cached project addon list from disk.
func (c *Ctx) RefreshProject() { c.loadProject() }

// SetUpdateChecks caches the latest per-addon update-check results for the
// Project list to render.
func (c *Ctx) SetUpdateChecks(m map[string]addon.UpdateInfo) { c.UpdateChecks = m }

// Scan resolves the project's paths from the project root: it walks for the addon
// manifest and derives the display fields (ManifestRel, ProjectName, HasProject). It's
// the single source of path state, run synchronously at construction (New) and — via
// RefreshPaths — after the manifest is created or otherwise changes. A missing manifest
// leaves ManifestPath/ManifestRel empty (the header shows a bootstrap hint).
func (c *Ctx) Scan() {
	c.ManifestPath, _ = addon.FindManifest(c.ProjectRoot)
	switch {
	case c.ManifestPath == "":
		c.ManifestRel = ""
	default:
		rel, err := filepath.Rel(c.ProjectRoot, c.ManifestPath)
		if err != nil {
			rel = c.ManifestPath
		}
		c.ManifestRel = rel
	}
	c.ProjectName, c.HasProject = addon.ProjectName(c.ProjectRoot)
}

// Of recovers the gdaddon context from a Shared. Tabs call c := appctx.Of(sh) to
// reach ManifestPath/ProjectRoot.
func Of(sh *core.Shared) *Ctx { return core.App[Ctx](sh) }

// LockToggle flips the lock on name in the manifest at path and returns the new lock
// state plus the past-tense verb ("locked"/"unlocked"). A SetLock error is returned
// as-is (callers wrap it with core.StatusErr); the status line, dirty payload, and
// rebuilt submenu differ between the project and set lock toggles and stay at the
// call site.
func LockToggle(path, name string, cur bool) (newLock bool, verb string, err error) {
	newLock = !cur
	if e := addon.SetLock(path, name, newLock); e != nil {
		return false, "", e
	}
	verb = "locked"
	if !newLock {
		verb = "unlocked"
	}
	return newLock, verb, nil
}

// selfUpdateCheckTimeout caps the startup self-update check's network fetch so a slow
// or unreachable host can't leave it pending.
const selfUpdateCheckTimeout = 30 * time.Second

// SelfUpdateCheckCmd is the app-level startup command (wired onto bubblestack
// Config.Init): it checks gdaddon's own repo for a newer release off the UI thread
// and, only when an update is available, writes an "update available" line to the
// shared status line and log. Anything else (up to date, dev build, fetch error) is
// silent. The returned Action rides back on the cmd's tea.Msg and is applied by the
// router, the same pattern as the project tab's update check.
func SelfUpdateCheckCmd(sh *core.Shared) tea.Cmd {
	version := Of(sh).Version
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), selfUpdateCheckTimeout)
		defer cancel()
		info, err := selfupdate.Check(ctx, version)
		if err != nil || !info.Available {
			return nil
		}
		return core.SetStatusAndLog(
			fmt.Sprintf("gdaddon update available: %s → %s · Actions ▸ Update gdaddon", info.Current, info.LatestTag),
			true,
		)
	}
}

// Receive handles App-level broadcasts (the router notifies App on every PropagateAll).
// A theme change rebuilds the cached tab roots so each re-bakes its delegate/list styles
// with the new palette — gdaddon is fine reinstancing roots (they reflect on-disk state,
// see RefreshProject/RefreshGlobal/RefreshArchive). Other payloads (the Dirty markers)
// are handled by the individual tab roots, so App ignores them.
func (c *Ctx) Receive(sh *core.Shared, payload any) core.Action {
	switch payload.(type) {
	case core.MsgThemeChanged:
		return core.RefreshRoots()
	}
	return core.Action{}
}

// Tab titles, shared between the TabEntry wiring (in Run) and the ShowTab callers,
// so the focus-grab in a Receive never duplicates a raw title literal that a rename
// could silently desync.
const (
	TitleProject = "Project"
	TitleGlobal  = "Global"
	TitleSets    = "Sets"
	TitleArchive = "Archive"
	TitleActions = "Actions"
	TitleSearch  = "Search"
)

// Dirty payloads are broadcast via core.PropagateAll after an out-of-band change.
// They are pure "reload yourself" markers: the matching tab root recognizes its own
// payload in Receive and reloads from disk. The visible outcome — the status line and
// any focus switch — is composed at the call site with core.Seq (SetStatus / ShowTab
// alongside the PropagateAll), so the payload carries no state.
type (
	ProjectDirty struct{}
	GlobalDirty  struct{}
	ArchiveDirty struct{}
	// SetsDirty is broadcast after a set is created or deleted, so the pushed Sets
	// submenu (Actions ▸ Sets) reloads its list from ~/.gdaddon/sets.
	SetsDirty struct{}
	// PathRefresh is broadcast after the manifest/project paths themselves change (e.g.
	// a manifest was just created). Path-dependent roots — the Project list and the
	// Actions menu — reload from the updated context; the header needs no notification
	// (it reads straight from App each render).
	PathRefresh struct{}
)

// RefreshPaths re-runs Scan after the paths may have changed (e.g. a manifest was just
// created). When async it defers the scan into a tea.Cmd that, once it runs, emits the
// PathRefresh broadcast — so the scan completes before any Receiver (or the
// live-reading header) reacts, with no router ordering or chrome plumbing. When sync it
// just re-scans inline and returns no broadcast, for callers that run before anything
// needs to reload. The status/focus that used to ride the broadcast are now composed
// at the call site (core.Seq), so RefreshPaths only carries the reload.
func RefreshPaths(sh *core.Shared, async bool) tea.Cmd {
	if async {
		return func() tea.Msg {
			Of(sh).Scan()
			return core.PropagateAll(PathRefresh{})
		}
	}
	Of(sh).Scan()
	return nil
}

// Header renders gdaddon's persistent context box (Project / Root / Manifest). It is
// wired onto core.Chrome.Header in Run, so the agnostic router draws it on every
// screen without naming any domain type.
func Header(sh *core.Shared) string {
	c := Of(sh)
	name := "No Project File"
	if c.HasProject {
		name = c.ProjectName
		if name == "" {
			name = "(unnamed project)"
		}
	}
	inner := core.HeaderInnerWidth(sh.Width())
	valWidth := inner - 10 // minus the "Manifest: " label
	line := func(label, value string) string {
		return core.Label(label) + core.Value(core.TruncLeft(value, valWidth))
	}
	manifest := c.ManifestRel
	if manifest == "" {
		manifest = "(none — Actions ▸ Create manifest)"
	}
	body := strings.Join([]string{
		core.Label("Project:  ") + core.Value(name),
		line("Root:     ", c.ProjectRoot),
		line("Manifest: ", manifest),
	}, "\n")
	return core.HeaderBox(sh.Width(), body)
}

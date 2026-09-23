// Package appctx is gdaddon's TUI context: project paths, the header, tab titles and the
// Dirty payloads. A leaf package, so the tui package and the tabs can both use it.
package appctx

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/archive"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"
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

	// Shared local inspection used by row rendering and git-menu scopes.
	projectStatuses []addon.Status
	projectLoaded   bool

	// LastSearchQuery keeps the search form filled across navigation (session only).
	LastSearchQuery string

	// Compact is the session list density shared through ListDensity (true is one row per
	// item), saved by bubblestack.Run.
	Compact bool

	// UpdateChecks caches per-addon update results by name, filled asynchronously and read by
	// the Project list's marker. Missing keys mean UpdateUnknown.
	UpdateChecks map[string]addon.UpdateInfo

	// DepStatuses caches each addon's declared dependencies and their state by name
	// (recomputed locally in loadProject), for the missing-deps marker, the Dependencies row
	// and screen.
	DepStatuses map[string][]addon.DepStatus

	// GitDirty marks clone entries with uncommitted changes, by name, recomputed in
	// loadProject.
	GitDirty map[string]bool

	// GitSync caches each tracking checkout's divergence, by name, from local refs; new
	// upstream commits appear only after a fetch.
	GitSync map[string]addon.GitSync

	// OrphanDeps marks is_dependency entries nothing installed requires, by name.
	OrphanDeps map[string]bool

	// RootRepo is the project root's own checkout (nil if none), re-described each loadProject
	// for the header, the fetch key and the batch menu's include-root toggle.
	RootRepo *repo.Repo

	// loadErrs collects real load failures (not missing files) for the UI to show.
	loadErrs []string
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
	addons, err := addon.Parse(p)
	c.noteLoadErr("global list", err)
	c.GlobalAddons = addons
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
	c.projectLoaded = true
	c.projectStatuses = nil
	// Describe the root repo even without a manifest, so the header stays current.
	if root, ok := repo.DescribeRoot(c.ProjectRoot); ok {
		c.RootRepo = &root
	} else {
		c.RootRepo = nil
	}
	if c.ManifestPath == "" {
		c.ProjectAddons = nil
		c.DepStatuses = nil
		c.OrphanDeps = nil
		c.GitDirty = nil
		c.GitSync = nil
		return
	}
	addons, err := addon.Parse(c.ManifestPath)
	c.noteLoadErr("manifest", err)
	c.ProjectAddons = addons
	// Inspect the parsed entries once and share the statuses between caches and
	// row rendering. A failed parse leaves no statuses to inspect.
	var statuses []addon.Status
	if err == nil {
		for _, a := range addons {
			statuses = append(statuses, addon.InspectOne(a, c.ProjectRoot))
		}
	}
	c.projectStatuses = statuses
	c.refreshDepChecks(statuses)
	c.OrphanDeps = addon.OrphanDeps(statuses)
	c.refreshGitChecks(statuses)
}

// refreshDepChecks recomputes each addon's dependency states from the inspection (local
// and cheap).
func (c *Ctx) refreshDepChecks(statuses []addon.Status) {
	checks := make(map[string][]addon.DepStatus)
	for _, a := range c.ProjectAddons {
		if ds, err := addon.DepStatuses(a, c.ProjectRoot, statuses); err == nil && len(ds) > 0 {
			checks[a.Name] = ds
		}
	}
	c.DepStatuses = checks
}

// RefreshGlobal reloads the cached global addon list from disk.
func (c *Ctx) RefreshGlobal() { c.loadGlobal() }

// RefreshArchive reloads the cached archived repo IDs from disk.
func (c *Ctx) RefreshArchive() { c.loadArchive() }

// RefreshProject reloads the cached project addon list from disk.
func (c *Ctx) RefreshProject() { c.loadProject() }

// ProjectStatuses returns the cached inspection, loading it once. Treat it as read-only.
func (c *Ctx) ProjectStatuses() []addon.Status {
	if !c.projectLoaded {
		c.loadProject()
	}
	return c.projectStatuses
}

// noteLoadErr records a failure for DrainLoadErrs; a missing file is a valid empty state
// and is ignored.
func (c *Ctx) noteLoadErr(what string, err error) {
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return
	}
	c.loadErrs = append(c.loadErrs, what+": "+err.Error())
}

// DrainLoadErrs returns and clears the recorded failures; Receive and the startup hook
// show them.
func (c *Ctx) DrainLoadErrs() []string {
	errs := c.loadErrs
	c.loadErrs = nil
	return errs
}

// SetUpdateChecks caches the latest per-addon update-check results for the
// Project list to render.
func (c *Ctx) SetUpdateChecks(m map[string]addon.UpdateInfo) { c.UpdateChecks = m }

// Scan finds the manifest under the project root and derives the display fields, at
// construction and via RefreshPaths. A missing manifest leaves them empty (the header
// shows a hint).
func (c *Ctx) Scan() {
	path, err := addon.FindManifest(c.ProjectRoot)
	c.noteLoadErr("manifest search", err)
	c.ManifestPath = path
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

// Receive handles App broadcasts: a theme change rebuilds the tab roots
// (core.OnThemeChange); Dirty markers are left to the roots. Load failures recorded by the
// reloads are shown here, since App is notified last.
func (c *Ctx) Receive(sh *core.Shared, payload any) core.Action {
	acts := []core.Action{core.OnThemeChange(payload)}
	for _, e := range c.DrainLoadErrs() {
		acts = append(acts, core.SetStatusAndLog(e))
	}
	return core.Seq(acts...)
}

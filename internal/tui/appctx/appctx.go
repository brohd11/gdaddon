// Package appctx is gdaddon's TUI context: project paths, the header, tab titles and the
// Dirty payloads. A leaf package, so the tui package and the tabs can both use it.
package appctx

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/source"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"
	"github.com/brohd11/gitstack/repoui"
)

// Ctx is the consumer context stored on core.Shared.App. Tabs recover it with Of.
type Ctx struct {
	Version       string // the running binary's version, for the self-update check
	ManifestPath  string
	SearchRoot    string // stable discovery directory, independent of manifest settings
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
	projectIndex    map[string]int

	// LastSearchQuery keeps the search form filled across navigation (session only).
	LastSearchQuery   string
	LastSearchOwner   string
	LastSearchLicense string
	LastSearchForks   bool

	// Compact is the session list density shared through ListDensity (true is one row per
	// item), saved by bubblestack.Run.
	Compact bool

	// GlobalFolds is the Global tree's fold state for the session, kept here because a theme
	// change reinstances every tab root. Use GlobalTreeFolds.
	GlobalFolds *components.TreeFolds
	// globalCategories maps source.RepoID to category, built lazily from GlobalAddons.
	globalCategories map[string]string

	// ProjectTree shows the Project tab as a category tree instead of the list (session
	// only; the b key flips it). ProjectFolds is that tree's fold state (use
	// ProjectTreeFolds). ProjectSort is the tab's sort mode, kept here so switching views
	// and theme rebuilds keep it.
	ProjectTree  bool
	ProjectFolds *components.TreeFolds
	ProjectSort  SortMode

	// UpdateChecks caches per-addon update results by name, filled asynchronously and read by
	// the Project list's marker. Missing keys mean UpdateUnknown.
	UpdateChecks map[string]addon.UpdateInfo
	// UpdateGen numbers update checks so one superseded before it starts skips its fetches.
	UpdateGen atomic.Int64
	// pinBaseline is each entry's addon.UpdatePinKey as of the last async reload, and
	// pinChanged the entries whose key changed since, awaiting a targeted update re-check
	// (TakePinChanged). Kept apart from projectStatuses because a synchronous
	// RefreshProject can re-read the manifest before the reload compares.
	pinBaseline map[string]string
	pinChanged  []addon.Addon

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

	// RootRepo is the checkout enclosing the project root (nil if none), re-described each loadProject
	// for the header, the fetch key and the batch menu's include-root toggle.
	RootRepo *repo.Repo
	// Fetch belongs to the app session, surviving root rebuilds and navigation.
	Fetch repoui.FetchState

	// Refresh tracks the async project reload (StartProjectRefresh). It lives here, not on
	// the Project root, because roots are rebuilt while results are in flight.
	Refresh components.Refresh
	// DirtyProjectRows records accepted inspections waiting for list publication.
	ListUpdates      components.UpdateBatch
	DirtyProjectRows map[string]bool
	// Pending marks entries, by name, whose inspection the running Refresh is re-reading;
	// their rows show the cached state muted. RootPending is the same for RootRepo.
	Pending     map[string]bool
	RootPending bool
	// unseen marks pending entries with no earlier inspection to show (see Unseen).
	unseen map[string]bool
	// RankSnap holds the Project list's status-sort ranks, by name, from before the running
	// refresh, so rows keep their order until it lands. The project tab fills it; it is
	// cleared when the pass completes.
	RankSnap map[string]int

	// loadErrs collects real load failures (not missing files) for the UI to show.
	loadErrs []string
}

// New builds the context for a project root and performs the initial path scan and a
// full, synchronous project load.
func New(projectRoot, version string) *Ctx {
	c := newCtx(projectRoot, version)
	c.loadProject()
	return c
}

// NewDeferred is New without inspecting the project: the manifest is parsed and every
// entry left Pending, so the TUI can draw at once and inspect through
// StartProjectRefresh (broadcast GitRefresh to start it).
func NewDeferred(projectRoot, version string) *Ctx {
	c := newCtx(projectRoot, version)
	c.beginProject()
	return c
}

func newCtx(projectRoot, version string) *Ctx {
	c := &Ctx{SearchRoot: projectRoot, ProjectRoot: projectRoot, Version: version}
	c.Scan()
	c.loadGlobal()
	c.loadArchive()
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
	c.globalCategories = nil // rebuilt from GlobalAddons on the next GlobalCategory
}

// CategoryFor is a project entry's effective category: its own (an override) when set,
// else its global entry's.
func (c *Ctx) CategoryFor(a addon.Addon) string {
	if own := addon.NormalizeCategory(a.Category); own != "" {
		return own
	}
	return c.GlobalCategory(a.URL)
}

// GlobalCategory is the category of the global entry for url's repo (matched by
// source.RepoID, so url forms differ freely), or "" when it has none or isn't global.
// Categories are set only in the global list; the Project tree reads them here.
func (c *Ctx) GlobalCategory(url string) string {
	if c.globalCategories == nil {
		c.globalCategories = make(map[string]string, len(c.GlobalAddons))
		for _, a := range c.GlobalAddons {
			if id, err := source.RepoID(a.URL); err == nil && a.Category != "" {
				c.globalCategories[id] = addon.NormalizeCategory(a.Category)
			}
		}
	}
	id, err := source.RepoID(url)
	if err != nil {
		return ""
	}
	return c.globalCategories[id]
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
	addons, err := c.readProject()
	c.noteLoadErr("manifest", err)
	defer c.indexProject()
	c.ListUpdates.Reset()
	c.DirtyProjectRows = make(map[string]bool)
	c.projectLoaded = true
	c.projectStatuses = nil
	c.Pending, c.unseen, c.RootPending = nil, nil, false
	// Describe the root repo even without a manifest, so the header stays current.
	c.RootRepo = describeProjectRepo(c.ProjectRoot)
	if c.ManifestPath == "" {
		c.ProjectAddons = nil
		c.DepStatuses = nil
		c.OrphanDeps = nil
		c.GitDirty = nil
		c.GitSync = nil
		return
	}
	c.ProjectAddons = addons
	// Inspect the parsed entries once and share the statuses between caches and
	// row rendering. A failed parse leaves no statuses to inspect.
	var statuses []addon.Status
	if err == nil {
		if c.pinBaseline == nil {
			c.pinBaseline = pinKeys(addons)
		}
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
		ds, err := addon.DepStatuses(a, c.ProjectRoot, statuses)
		c.noteLoadErr(a.Label()+" dependencies", err)
		if err == nil && len(ds) > 0 {
			checks[a.Name] = ds
		}
	}
	c.DepStatuses = checks
}

// RefreshGlobal reloads the cached global addon list from disk.
func (c *Ctx) RefreshGlobal() { c.loadGlobal() }

// ProjectTreeFolds returns the session's Project tree fold state, creating it on first use.
func (c *Ctx) ProjectTreeFolds() *components.TreeFolds {
	if c.ProjectFolds == nil {
		c.ProjectFolds = components.NewTreeFolds()
	}
	return c.ProjectFolds
}

// GlobalTreeFolds returns the session's Global tree fold state, creating it on first use.
func (c *Ctx) GlobalTreeFolds() *components.TreeFolds {
	if c.GlobalFolds == nil {
		c.GlobalFolds = components.NewTreeFolds()
	}
	return c.GlobalFolds
}

// RefreshArchive reloads the cached archived repo IDs from disk.
func (c *Ctx) RefreshArchive() { c.loadArchive() }

// RefreshProject reloads the cached project addon list from disk, synchronously. The
// Project tab reloads through StartProjectRefresh instead; this is for callers that need
// the fresh state before they continue.
func (c *Ctx) RefreshProject() { c.loadProject() }

// ProjectStatuses returns the cached inspection, loading it once. Treat it as read-only.
func (c *Ctx) ProjectStatuses() []addon.Status {
	if !c.projectLoaded {
		c.loadProject()
	}
	return c.projectStatuses
}

func (c *Ctx) indexProject() {
	c.projectIndex = make(map[string]int, len(c.projectStatuses))
	for i, s := range c.projectStatuses {
		c.projectIndex[s.Addon.Name] = i
	}
}

// ProjectStatus looks up one cached inspection without scanning the project slice.
func (c *Ctx) ProjectStatus(name string) (addon.Status, bool) {
	statuses := c.ProjectStatuses()
	i, ok := c.projectIndex[name]
	if !ok {
		return addon.Status{}, false
	}
	return statuses[i], true
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

// MergeUpdateChecks stores re-check results for entries whose pin still has the key that
// was checked (keys by name); a result for an entry edited again since is dropped. It
// reports whether anything was stored.
func (c *Ctx) MergeUpdateChecks(checks map[string]addon.UpdateInfo, keys map[string]string) bool {
	stored := false
	for name, info := range checks {
		i, ok := c.projectIndex[name]
		if !ok || addon.UpdatePinKey(c.projectStatuses[i].Addon) != keys[name] {
			continue
		}
		if c.UpdateChecks == nil {
			c.UpdateChecks = make(map[string]addon.UpdateInfo)
		}
		c.UpdateChecks[name] = info
		stored = true
	}
	return stored
}

// TakePinChanged returns the entries whose update pin changed since the previous reload
// and clears the list.
func (c *Ctx) TakePinChanged() []addon.Addon {
	changed := c.pinChanged
	c.pinChanged = nil
	return changed
}

// notePins compares addons with pinBaseline: an entry that is new or whose pin changed
// loses its cached update result (the marker described the old pin) and is queued for a
// targeted re-check; results for removed entries are dropped. The first call only records
// the baseline, leaving the initial check to the daily one.
func (c *Ctx) notePins(addons []addon.Addon) {
	keys := pinKeys(addons)
	if c.pinBaseline == nil {
		c.pinBaseline = keys
		return
	}
	for _, a := range addons {
		if c.pinBaseline[a.Name] == keys[a.Name] {
			continue
		}
		delete(c.UpdateChecks, a.Name)
		c.pinChanged = slices.DeleteFunc(c.pinChanged, func(p addon.Addon) bool { return p.Name == a.Name })
		c.pinChanged = append(c.pinChanged, a)
	}
	for name := range c.UpdateChecks {
		if _, ok := keys[name]; !ok {
			delete(c.UpdateChecks, name)
		}
	}
	c.pinBaseline = keys
}

func pinKeys(addons []addon.Addon) map[string]string {
	keys := make(map[string]string, len(addons))
	for _, a := range addons {
		keys[a.Name] = addon.UpdatePinKey(a)
	}
	return keys
}

// Scan finds the manifest under the original search directory and derives the display fields, at
// construction and via RefreshPaths. A missing manifest leaves them empty (the header
// shows a hint).
func (c *Ctx) Scan() {
	if c.SearchRoot == "" {
		c.SearchRoot = c.ProjectRoot
	}
	path, err := addon.FindManifest(c.SearchRoot)
	c.applyScan(path, err)
}

func (c *Ctx) applyScan(path string, err error) {
	c.noteLoadErr("manifest search", err)
	c.ManifestPath = path
	_, err = c.readProject()
	c.noteLoadErr("manifest", err)
}

// readProject reloads root settings whenever entries are refreshed, including
// hand edits. Discovery always stays anchored to SearchRoot.
func (c *Ctx) readProject() ([]addon.Addon, error) {
	if c.SearchRoot == "" {
		c.SearchRoot = c.ProjectRoot
	}
	root := c.SearchRoot
	var addons []addon.Addon
	if c.ManifestPath != "" {
		m, err := addon.LoadManifest(c.ManifestPath)
		if err != nil {
			return nil, err
		}
		root, addons = m.ProjectRoot, m.Addons
	}
	if root != c.ProjectRoot {
		// Retire work tied to the old directory before publishing the new root.
		c.Refresh.Start(0)
		c.UpdateGen.Add(1)
		c.ProjectRoot = root
		c.projectLoaded = false
		c.ProjectAddons, c.projectStatuses, c.projectIndex = nil, nil, nil
		c.Pending, c.unseen = nil, nil
		c.GitDirty, c.GitSync = nil, nil
		c.DepStatuses, c.OrphanDeps, c.UpdateChecks = nil, nil, nil
		c.RootRepo, c.RootPending, c.RankSnap = nil, false, nil
		c.pinBaseline, c.pinChanged = nil, nil
	}
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
	return addons, nil
}

// describeProjectRepo keeps root Git actions on the enclosing checkout when the
// Godot project lives in a repository subdirectory.
func describeProjectRepo(projectRoot string) *repo.Repo {
	if root, ok := repo.DescribeRoot(projectRoot); ok {
		return &root
	}
	if dir, ok := repo.RepoRoot(projectRoot); ok {
		if root, ok := repo.DescribeRoot(dir); ok {
			return &root
		}
	}
	return nil
}

// Of recovers the gdaddon context from a Shared. Tabs call c := appctx.Of(sh) to
// reach ManifestPath/ProjectRoot.
func Of(sh *core.Shared) *Ctx { return core.App[Ctx](sh) }

// Receive handles App broadcasts: a theme change rebuilds the tab roots
// (core.OnThemeChange); Dirty markers are left to the roots. Load failures recorded by the
// reloads are shown here, since App is notified last.
func (c *Ctx) Receive(sh *core.Shared, payload any) core.Action {
	acts := []core.Action{core.OnThemeChange(payload)}
	if paths, ok := payload.(pathsFound); ok {
		c.applyScan(paths.path, paths.err)
		acts = append(acts, core.PropagateAll(PathRefresh{}))
	}
	for _, e := range c.DrainLoadErrs() {
		acts = append(acts, core.SetStatusAndLog(e))
	}
	return core.Seq(acts...)
}

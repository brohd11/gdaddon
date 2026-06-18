// Package appctx holds gdaddon's domain-specific TUI context: the manifest/project
// paths the tabs operate on, the persistent header that renders them, the tab titles,
// and the Dirty notification payloads the tab roots react to. It is the one place that wires
// the domain to the otherwise agnostic core/components framework — it lives in its
// own leaf package so both the tui package (which imports the tabs) and the tabs
// can read the context without an import cycle.
package appctx

import (
	"path/filepath"
	"strings"

	"gdaddon/internal/addon"

	"github.com/brohd11/bubblestack/core"
	tea "github.com/charmbracelet/bubbletea"
)

// Ctx is the consumer context stored on core.Shared.App. Tabs recover it with Of.
type Ctx struct {
	ManifestPath string
	ProjectRoot  string
	ManifestRel  string // ManifestPath relative to ProjectRoot, for display
	ProjectName  string
	HasProject   bool
}

// New builds the context for a project root and performs the initial path scan.
func New(projectRoot string) *Ctx {
	c := &Ctx{ProjectRoot: projectRoot}
	c.Scan()
	return c
}

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

// Tab titles, shared between the TabEntry wiring (in Run) and the ShowTab callers,
// so the focus-grab in a Receive never duplicates a raw title literal that a rename
// could silently desync.
const (
	TitleProject = "Project"
	TitleGlobal  = "Global"
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
		return core.Label(label) + core.TruncLeft(value, valWidth)
	}
	manifest := c.ManifestRel
	if manifest == "" {
		manifest = "(none — Actions ▸ Create manifest)"
	}
	body := strings.Join([]string{
		core.Label("Project:  ") + name,
		line("Root:     ", c.ProjectRoot),
		line("Manifest: ", manifest),
	}, "\n")
	return core.HeaderBox(sh.Width(), body)
}

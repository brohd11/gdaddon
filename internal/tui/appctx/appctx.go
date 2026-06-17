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
)

// Ctx is the consumer context stored on core.Shared.App. Tabs recover it with Of.
type Ctx struct {
	ManifestPath string
	ProjectRoot  string
	ManifestRel  string // ManifestPath relative to ProjectRoot, for display
	ProjectName  string
	HasProject   bool
}

// New builds the context from the resolved manifest + project paths, deriving the
// display-relative manifest path and the project name (the work core.NewShared used
// to do before the framework was made domain-agnostic).
func New(manifestPath, projectRoot string) *Ctx {
	rel, err := filepath.Rel(projectRoot, manifestPath)
	if err != nil {
		rel = manifestPath
	}
	name, exists := addon.ProjectName(projectRoot)
	return &Ctx{
		ManifestPath: manifestPath,
		ProjectRoot:  projectRoot,
		ManifestRel:  rel,
		ProjectName:  name,
		HasProject:   exists,
	}
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

// Dirty payloads are broadcast via core.PropagateAll after an out-of-band change. The
// matching tab root recognizes its own payload in Receive, reloads from disk, sets
// Status, and — when Focus is set — returns core.ShowTab to make itself active. Focus
// is per-event so the same payload can both grab focus (acted on in that tab) or
// reload silently (a side effect of an action in another tab).
type (
	ProjectDirty struct {
		Status string
		Focus  bool
	}
	GlobalDirty struct {
		Status string
		Focus  bool
	}
	ArchiveDirty struct {
		Status string
		Focus  bool
	}
)

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
	body := strings.Join([]string{
		core.Label("Project:  ") + name,
		line("Root:     ", c.ProjectRoot),
		line("Manifest: ", c.ManifestRel),
	}, "\n")
	return core.HeaderBox(sh.Width(), body)
}

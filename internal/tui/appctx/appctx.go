// Package appctx holds gdaddon's domain-specific TUI context: the manifest/project
// paths the tabs operate on, the persistent header that renders them, and the
// refresh-target identifiers the tab roots claim. It is the one place that wires
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

// RefreshTarget names which tab root a core.MsgRefresh is meant for. The framework
// treats it as an opaque identifier; each root compares against these in HandleRoot.
type RefreshTarget int

const (
	Project RefreshTarget = iota // the browse/project addon list
	Global                       // the global plugin list
	Archive                      // the local package archive
)

// Header renders gdaddon's persistent context box (Project / Root / Manifest). It is
// wired onto core.Shared.Header in Run, so the agnostic router draws it on every
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

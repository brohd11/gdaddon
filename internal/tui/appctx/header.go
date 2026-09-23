package appctx

import (
	"strings"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repoui"
)

// Header renders gdaddon's context box (Project, Root, Manifest) for core.Chrome.Header.
func Header(sh *core.Shared) string {
	c := Of(sh)
	name := "No Project File"
	if c.HasProject {
		name = c.ProjectName
		if name == "" {
			name = "(unnamed project)"
		}
	}
	valWidth := core.HeaderValueWidth(sh.Width(), "Manifest: ")
	line := func(label, value string) string {
		return core.Label(label) + core.Value(core.TruncLeft(value, valWidth))
	}
	manifest := c.ManifestRel
	if manifest == "" {
		manifest = "(none — Actions ▸ Create manifest)"
	}
	// The root repo's status marker follows the path; RootLineValue makes room for it.
	rootValue := repoui.RootLineValue(c.ProjectRoot, c.RootRepo, valWidth)
	body := strings.Join([]string{
		core.Label("Project:  ") + core.Value(name),
		core.Label("Root:     ") + core.Value(rootValue),
		line("Manifest: ", manifest),
	}, "\n")
	return core.HeaderBox(sh.Width(), body)
}

package appctx

import (
	"strings"
	"unicode/utf8"

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
	if c.RootPending {
		// Still being re-read: the cached marker plus a muted note, when it fits.
		const note = " · refreshing…"
		rootValue = repoui.RootLineValue(c.ProjectRoot, c.RootRepo, max(valWidth-utf8.RuneCountInString(note), 0))
		rootValue = core.Value(rootValue) + core.Label(note)
	} else {
		rootValue = core.Value(rootValue)
	}
	body := strings.Join([]string{
		core.Label("Project:  ") + core.Value(name),
		core.Label("Root:     ") + rootValue,
		line("Manifest: ", manifest),
	}, "\n")
	return core.HeaderBox(sh.Width(), body)
}

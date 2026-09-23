package actions

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
)

// newCreateManifestForm asks for a directory (empty: the project root), writes an empty
// addon_manifest.yml there once it is within discovery depth, points the context at it and
// broadcasts so the lists reload. Only reachable without a manifest.
func newCreateManifestForm(sh *core.Shared) *components.FormScreen {
	root := appctx.Of(sh).ProjectRoot
	dirF := components.NewTextField("dir", "Dir:  ", "(optional — defaults to the project root)")

	return components.NewForm(components.FormOpts{
		Crumb: "Create Manifest",
		Fields: []components.FormField{
			components.NewHeading("Create manifest"),
			components.NewNote("addon_manifest.yml is created in this directory (blank ⇒ project root)."),
			components.NewSpacer(),
			dirF,
		},
		Focus: "dir",
		Help: []key.Binding{
			core.Hint("create", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		},
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			dir := strings.TrimSpace(f.Value("dir"))
			switch {
			case dir == "":
				dir = root
			case !filepath.IsAbs(dir):
				dir = filepath.Join(root, dir)
			}
			if !addon.WithinManifestDepth(root, dir) {
				return core.Seq(
					core.SetStatusAndLog(fmt.Sprintf("path must be inside the project (within %d dirs of the root)", addon.MaxManifestDepth)),
					core.Async(f.Focus("dir")),
				)
			}
			target := filepath.Join(dir, "addon_manifest.yml")
			if err := addon.CreateManifest(target); err != nil {
				return core.SeqErr(err, core.Async(f.Focus("dir")))
			}
			// Show the Project tab and asynchronously re-scan the paths, which finds the new manifest
			// and broadcasts PathRefresh.
			return core.Seq(
				core.SetStatusAndLog("Created Manifest: "+target),
				core.ShowTab(appctx.TitleProject),
				core.Async(appctx.RefreshPaths(sh, true)),
			)
		},
	})
}

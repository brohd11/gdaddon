package actions

import (
	"fmt"
	"path/filepath"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
)

// newCreateManifestForm builds the Create-manifest form: one directory field whose
// value gets addon_manifest.yml appended (empty ⇒ the project root). On submit it
// validates the dir is reachable from the root, writes an empty manifest, points the
// context at it, and broadcasts so the Project list reloads (focused) and this menu
// drops the Create row. Only reached while no manifest is loaded.
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
			// Compose the outcome at the call site: set the status, show the Project tab
			// (ShowTab discards this form's stack), and async-refresh the paths — the
			// refresh re-scans under the root, rediscovers the file we just wrote
			// (validated above to be within the walk depth), then broadcasts PathRefresh
			// so the Project list and Actions menu reload.
			return core.Seq(
				core.SetStatusAndLog("Created Manifest: "+target),
				core.ShowTab(appctx.TitleProject),
				core.Async(appctx.RefreshPaths(sh, true)),
			)
		},
	})
}

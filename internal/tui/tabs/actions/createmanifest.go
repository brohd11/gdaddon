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
		Crumb: core.RenderTitleBar("Create manifest"),
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
				sh.SetStatus(fmt.Sprintf("path must be inside the project (within %d dirs of the root)", addon.MaxManifestDepth))
				return core.Async(f.Focus("dir"))
			}
			target := filepath.Join(dir, "addon_manifest.yml")
			if err := addon.CreateManifest(target); err != nil {
				sh.SetStatus("error: " + err.Error())
				return core.Async(f.Focus("dir"))
			}
			// The async refresh re-scans under the root and rediscovers the file we just
			// wrote (validated above to be within the walk depth), then broadcasts.
			return core.Seq(core.ResetToRoot(), appctx.RefreshPaths(sh, true, "created manifest", true))
		},
	})
}

// Package newplugin is the Add Plugin flow (form, confirm, and the write to the project
// manifest or global list), opened from Actions and from Search with a prefilled url.
package newplugin

import (
	"github.com/brohd11/gdaddon/internal/addon"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
)

// add targets for the target toggle (also the ToggleField option order).
const (
	targetProject = iota
	targetGlobal
)

// targetOptions is the Project/Global toggle's options, indexed by the target* consts.
var targetOptions = []string{"Project", "Global"}

// NewNewPluginForm builds an empty Add Plugin form (focus on the URL field).
func NewNewPluginForm() *components.FormScreen { return NewWithURL("") }

// NewWithURL builds the Add Plugin form with url prefilled and focus on Name; empty url
// behaves like NewNewPluginForm.
func NewWithURL(url string) *components.FormScreen {
	target := components.NewToggleField("target", "Add to:  ", targetOptions, "|")

	focus := "url"
	if url != "" {
		focus = "name"
	}

	return newAddonForm(formSpec{
		crumb:          "New Plugin",
		heading:        "Add plugin",
		urlPlaceholder: "https://github.com/owner/repo",
		focus:          focus,
		toggleLabel:    "target",
		values:         map[string]string{"url": url},
		tail:           []components.FormField{target},
		onSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			return submitAddonForm(f, addon.NormalizeRepoURL, func(name, url, path string) core.Action {
				return core.Push(newNewPluginConfirm(name, url, path, target.Index()))
			})
		},
	})
}

// ---------- confirm ----------

var newPluginConfirmHelp = []key.Binding{
	core.Hint("target", core.Keys.Left, core.Keys.Right),
	core.Hint("add", core.Keys.Select),
	core.Hint("back", core.Keys.Back),
}

func newNewPluginConfirm(name, url, path string, addTarget int) *components.DialogScreen {
	return newTargetConfirm(addTarget,
		func(sh *core.Shared, target int) string {
			return confirmBody(sh, "Add plugin", name, "", url, path, addToLine(target))
		},
		func(sh *core.Shared, target int) core.Action { return commitNewPlugin(sh, name, url, path, target) })
}

// commitNewPlugin writes the pending entry to the project manifest or the global
// list, then unwinds to the root (rebuilding the Browse list for a project add).
func commitNewPlugin(sh *core.Shared, name, url, path string, addTarget int) core.Action {
	return commitAdd(sh, name, url, path, addTarget, func(manifestPath string) error {
		return addon.AddEntry(manifestPath, name, url, path)
	})
}

// otherTarget toggles between the Project and Global add targets.
func otherTarget(t int) int {
	if t == targetProject {
		return targetGlobal
	}
	return targetProject
}

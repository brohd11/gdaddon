package newplugin

import (
	"github.com/brohd11/gdaddon/internal/addon"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
)

// NewStoreForm builds the Add Store Asset form (store url prefilled, focus on Name, target
// toggle). The store url is kept as-is, and a project add pins the store release as the
// tag.
func NewStoreForm(url, version string) *components.FormScreen {
	target := components.NewToggleField("target", "Add to:  ", targetOptions, "|")

	return newAddonForm(formSpec{
		crumb:          "Store Asset",
		heading:        "Add store asset",
		urlPlaceholder: "https://store.godotengine.org/publisher/slug",
		focus:          "name",
		toggleLabel:    "target",
		values:         map[string]string{"url": url},
		tail: []components.FormField{
			target,
			components.NewNote("  release " + version),
		},
		onSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			return submitAddonForm(f, nil, func(name, url, path string) core.Action {
				return core.Push(newStoreConfirm(name, url, path, version, target.Index()))
			})
		},
	})
}

func newStoreConfirm(name, url, path, version string, addTarget int) *components.DialogScreen {
	return newTargetConfirm(addTarget,
		func(sh *core.Shared, target int) string {
			v := version // local copy: the "(unspecified)" label is display-only, never committed as the tag
			if v == "" {
				v = "(unspecified)"
			}
			return confirmBody(sh, "Add store asset", name, v, url, path, addToLine(target))
		},
		func(sh *core.Shared, target int) core.Action {
			return commitStoreAsset(sh, name, url, path, version, target)
		})
}

// commitStoreAsset writes the store entry to the project manifest (release as tag; the
// version comes from plugin.cfg on install) or to the global list (url only).
func commitStoreAsset(sh *core.Shared, name, url, path, version string, addTarget int) core.Action {
	return commitAdd(sh, name, url, path, addTarget, func(manifestPath string) error {
		return addon.AddEntryFull(manifestPath, addon.Addon{Name: name, URL: url, Path: path, Tag: version})
	})
}

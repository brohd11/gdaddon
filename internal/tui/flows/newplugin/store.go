package newplugin

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
)

// NewStoreForm builds the Add Store Asset form, mirroring NewWithURL: the canonical
// store url prefilled (focus on Name), editable name/path, and the Project/Global
// target toggle. It is store-aware on commit (commitStoreAsset): the store url is
// preserved as-is (never NormalizeRepoURL'd into a .git url), and a project add pins
// the store release version. The Search tab opens this for a chosen store asset.
func NewStoreForm(url, version string) *components.FormScreen {
	urlF := components.NewTextField("url", "URL:     ", "https://store.godotengine.org/publisher/slug")
	nameF := components.NewTextField("name", "Name:    ", "(optional — derived from url)")
	pathF := components.NewTextField("path", "Path:    ", "(optional — derived on install)")
	target := components.NewToggleField("target", "Add to:  ", targetOptions, "|")

	urlF.SetValue(url)

	return components.NewForm(components.FormOpts{
		Crumb: "Store Asset",
		Fields: []components.FormField{
			components.NewHeading("Add store asset"),
			components.NewSpacer(),
			urlF, nameF, pathF,
			components.NewSpacer(),
			target,
			components.NewNote("  release " + version),
		},
		Focus: "name",
		Help: []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint("target", core.Keys.Left, core.Keys.Right),
			core.Hint("next", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		},
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			u := strings.TrimSpace(f.Value("url"))
			if u == "" {
				return core.Async(f.Focus("url"))
			}
			name := strings.TrimSpace(f.Value("name"))
			if name == "" {
				name = addon.DeriveName(u)
			}
			path := strings.TrimSpace(f.Value("path"))
			return core.Push(newStoreConfirm(name, u, path, version, target.Index()))
		},
	})
}

func newStoreConfirm(name, url, path, version string, addTarget int) *components.DialogScreen {
	target := addTarget // local copy the toggle mutates
	return &components.DialogScreen{
		Render: func(sh *core.Shared) string { return sh.Box(storeConfirmBody(sh, name, url, path, version, target)) },
		OnKey: func(sh *core.Shared, k string) core.Action {
			if core.MatchKey(k, core.Keys.Left) || core.MatchKey(k, core.Keys.Right) {
				target = otherTarget(target)
			}
			return core.Action{}
		},
		OnYes: func(sh *core.Shared) core.Action { return commitStoreAsset(sh, name, url, path, version, target) },
		Help:  newPluginConfirmHelp,
	}
}

func storeConfirmBody(sh *core.Shared, name, url, path, version string, addTarget int) string {
	urlBlock := core.IndentLines(core.HardWrap(url, sh.ConfirmWidth()-4), "    ")
	if path == "" {
		path = "(derived on install)"
	}
	if version == "" {
		version = "(unspecified)"
	}
	return fmt.Sprintf(
		"Add store asset\n\n  name:     %s\n  version:  %s\n  url:\n%s\n  path:     %s\n\n  add to:   %s",
		name, version, urlBlock, path, components.RenderToggle(targetOptions, addTarget, ""))
}

// commitStoreAsset writes the store entry to the project manifest (pinning the store
// release version) or the global list (url-only, like a git global entry, so it can
// be imported into any project), then unwinds to the matching tab.
func commitStoreAsset(sh *core.Shared, name, url, path, version string, addTarget int) core.Action {
	if addTarget == targetGlobal {
		globalPath, err := addon.GlobalListPath()
		if err == nil {
			err = addon.AddEntry(globalPath, name, url, path)
		}
		if err != nil {
			return core.SeqErr(err, core.ResetToRoot())
		}
		return core.Seq(
			core.SetStatus(fmt.Sprintf("added %s to global list", name)),
			core.PropagateAll(appctx.GlobalDirty{}),
			core.ShowTab(appctx.TitleGlobal),
		)
	}

	if err := addon.AddEntryFull(appctx.Of(sh).ManifestPath, addon.Addon{Name: name, URL: url, Path: path, Version: version}); err != nil {
		return core.Seq(
			core.SetStatus("error: "+err.Error()),
			core.ResetToRoot(),
		)
	}
	return core.Seq(
		core.ResetToRoot(),
		core.SetStatus("added "+name),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.ShowTab(appctx.TitleProject),
	)
}

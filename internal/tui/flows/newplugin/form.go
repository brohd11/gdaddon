// Package newplugin is the shared "Add Plugin" flow: the url/name/path form, its
// confirm screen, and the commit that writes the entry to the project manifest or
// the global list. It lives outside any single tab because more than one tab opens
// it — the Actions tab ("New Plugin") and the Search tab (with the URL prefilled
// from a chosen asset). It sits in the flows layer between components and tabs
// (core ← components ← flows ← tabs ← tui), so tabs compose it without importing
// each other.
package newplugin

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/store"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
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

// NewWithURL builds the Add Plugin form (a generic components.FormScreen) with the
// URL prefilled (focus jumps to the Name field, since the URL is already known). An
// empty url behaves like NewNewPluginForm. The Search tab uses this to hand off a
// chosen asset's repo URL.
func NewWithURL(url string) *components.FormScreen {
	urlF := components.NewTextField("url", "URL:     ", "https://github.com/owner/repo")
	nameF := components.NewTextField("name", "Name:    ", "(optional — derived from url)")
	pathF := components.NewTextField("path", "Path:    ", "(optional — derived on install)")
	target := components.NewToggleField("target", "Add to:  ", targetOptions, "|")

	focus := "url"
	if url != "" {
		urlF.SetValue(url)
		focus = "name"
	}

	return components.NewForm(components.FormOpts{
		Crumb: "New Plugin",
		Fields: []components.FormField{
			components.NewHeading("Add plugin"),
			components.NewSpacer(),
			urlF, nameF, pathF,
			components.NewSpacer(),
			target,
		},
		Focus: focus,
		Help: []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint("target", core.Keys.Left, core.Keys.Right),
			core.Hint("next", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		},
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			url := strings.TrimSpace(f.Value("url"))
			if url == "" {
				return core.Async(f.Focus("url"))
			}
			name := strings.TrimSpace(f.Value("name"))
			if name == "" {
				name = addon.DeriveName(url)
			}
			path := strings.TrimSpace(f.Value("path"))
			return core.Push(newNewPluginConfirm(name, addon.NormalizeRepoURL(url), path, target.Index()))
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
	target := addTarget // local copy the toggle mutates
	return &components.DialogScreen{
		Render: func(sh *core.Shared) string { return sh.Box(newPluginConfirmBody(sh, name, url, path, target)) },
		OnKey: func(sh *core.Shared, k string) core.Action {
			if core.MatchKey(k, core.Keys.Left) || core.MatchKey(k, core.Keys.Right) {
				target = otherTarget(target)
			}
			return core.Action{}
		},
		OnYes: func(sh *core.Shared) core.Action { return commitNewPlugin(sh, name, url, path, target) },
		Help:  newPluginConfirmHelp,
	}
}

func newPluginConfirmBody(sh *core.Shared, name, url, path string, addTarget int) string {
	urlBlock := core.IndentLines(core.HardWrap(url, sh.ConfirmWidth()-4), "    ")
	if path == "" {
		path = "(derived on install)"
	}
	return fmt.Sprintf(
		"Add plugin\n\n  name:     %s\n  url:\n%s\n  path:     %s\n\n  add to:   %s",
		name, urlBlock, path, components.RenderToggle(targetOptions, addTarget, ""))
}

// commitNewPlugin writes the pending entry to the project manifest or the global
// list, then unwinds to the root (rebuilding the Browse list for a project add).
func commitNewPlugin(sh *core.Shared, name, url, path string, addTarget int) core.Action {
	if addTarget == targetGlobal {
		globalPath, err := addon.GlobalListPath()
		if err == nil {
			err = addon.AddEntry(globalPath, name, url, path)
		}
		if err != nil {
			return core.Seq(
				core.SetStatusAndLog("error: "+err.Error()),
				core.ResetToRoot(),
			)
		}
		// Show the Global tab rebuilt with the new entry (parallel to a project add
		// switching to Browse).
		return core.Seq(
			core.SetStatus(fmt.Sprintf("added %s to global list", name)),
			core.PropagateAll(appctx.GlobalDirty{}),
			core.ShowTab(appctx.TitleGlobal),
		)
	}

	if err := addon.AddEntry(appctx.Of(sh).ManifestPath, name, url, path); err != nil {
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

// ---------- track installed (scan) ----------

var trackConfirmHelp = []key.Binding{
	core.Hint("add", core.Keys.Select),
	core.Hint("back", core.Keys.Back),
}

// NewFromInstall builds the form for tracking an already-installed plugin found by
// the Scan action: name and path are prefilled from disk and url is prefilled with a
// suggestion (a pathless manifest entry that looks like this folder, if any), with
// focus on the url field so the user pastes/confirms it. On submit it upserts the
// project entry — backfilling path/version on a matching pathless entry (the cogito
// case) or adding a new one — so a bundled/sideloaded plugin starts being tracked.
func NewFromInstall(path, name, version, suggestedURL string) *components.FormScreen {
	urlF := components.NewTextField("url", "URL:     ", "https://github.com/owner/repo")
	nameF := components.NewTextField("name", "Name:    ", "(optional — derived from url)")
	pathF := components.NewTextField("path", "Path:    ", "(optional — derived on install)")

	urlF.SetValue(suggestedURL)
	nameF.SetValue(name)
	pathF.SetValue(path)

	return components.NewForm(components.FormOpts{
		Crumb: "Track Plugin",
		Fields: []components.FormField{
			components.NewHeading("Track installed plugin"),
			components.NewSpacer(),
			urlF, nameF, pathF,
			components.NewSpacer(),
			components.NewNote("  installed " + versionLabel(version)),
		},
		Focus: "url",
		Help: []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint("next", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		},
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			url := strings.TrimSpace(f.Value("url"))
			if url == "" {
				return core.Async(f.Focus("url"))
			}
			if !store.IsStoreURL(url) {
				url = addon.NormalizeRepoURL(url)
			}
			name := strings.TrimSpace(f.Value("name"))
			if name == "" {
				name = addon.DeriveName(url)
			}
			path := strings.TrimSpace(f.Value("path"))
			return core.Push(newTrackConfirm(name, url, path, version))
		},
	})
}

func newTrackConfirm(name, url, path, version string) *components.DialogScreen {
	return &components.DialogScreen{
		Render: func(sh *core.Shared) string { return sh.Box(trackConfirmBody(sh, name, url, path, version)) },
		OnYes:  func(sh *core.Shared) core.Action { return commitTrack(sh, name, url, path, version) },
		Help:   trackConfirmHelp,
	}
}

func trackConfirmBody(sh *core.Shared, name, url, path, version string) string {
	urlBlock := core.IndentLines(core.HardWrap(url, sh.ConfirmWidth()-4), "    ")
	if path == "" {
		path = "(derived on install)"
	}
	return fmt.Sprintf(
		"Track plugin\n\n  name:     %s\n  version:  %s\n  url:\n%s\n  path:     %s",
		name, versionLabel(version), urlBlock, path)
}

// commitTrack upserts the installed plugin into the project manifest: UpsertEntry
// matches by repo identity, so it backfills path+version on an existing pathless
// entry or appends a new one.
func commitTrack(sh *core.Shared, name, url, path, version string) core.Action {
	if err := addon.UpsertEntry(appctx.Of(sh).ManifestPath, name, url, path, version, ""); err != nil {
		return core.Seq(
			core.SetStatus("error: "+err.Error()),
			core.ResetToRoot(),
		)
	}
	return core.Seq(
		core.ResetToRoot(),
		core.SetStatus("tracking "+name),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.ShowTab(appctx.TitleProject),
	)
}

func versionLabel(version string) string {
	if version == "" {
		return "(version unknown)"
	}
	return "v" + version
}

// ---------- store asset form ----------

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
			return core.Seq(
				core.SetStatusAndLog("error: "+err.Error()),
				core.ResetToRoot(),
			)
		}
		return core.Seq(
			core.SetStatus(fmt.Sprintf("added %s to global list", name)),
			core.PropagateAll(appctx.GlobalDirty{}),
			core.ShowTab(appctx.TitleGlobal),
		)
	}

	if err := addon.AddEntryWithVersion(appctx.Of(sh).ManifestPath, name, url, path, version, ""); err != nil {
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

func otherTarget(t int) int {
	if t == targetProject {
		return targetGlobal
	}
	return targetProject
}

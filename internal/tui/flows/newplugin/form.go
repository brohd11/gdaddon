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
		Crumb: core.RenderTitleBar("New Plugin"),
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

func newNewPluginConfirm(name, url, path string, addTarget int) *components.ConfirmScreen {
	target := addTarget // local copy the toggle mutates
	return &components.ConfirmScreen{
		Crumb:  core.RenderTitleBar("New Plugin"),
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
			sh.SetStatus("error: " + err.Error())
			return core.ResetToRoot()
		}
		// Show the Global tab rebuilt with the new entry (parallel to a project add
		// switching to Browse).
		return core.PropagateAll(appctx.GlobalDirty{Status: fmt.Sprintf("added %s to global list", name), Focus: true})
	}

	if err := addon.AddEntry(appctx.Of(sh).ManifestPath, name, url, path); err != nil {
		sh.SetStatus("error: " + err.Error())
		return core.ResetToRoot()
	}
	return core.Seq(core.ResetToRoot(), core.PropagateAll(appctx.ProjectDirty{Status: "added " + name, Focus: true}))
}

func otherTarget(t int) int {
	if t == targetProject {
		return targetGlobal
	}
	return targetProject
}

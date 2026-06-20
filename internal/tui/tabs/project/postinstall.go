package project

import (
	"path/filepath"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
)

// global toggle options (index 0 = skip the global action, 1 = perform it).
const (
	globalSkip = iota
	globalDo
)

var locationFormHelp = []key.Binding{
	core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
	core.Hint("global", core.Keys.Left, core.Keys.Right),
	core.Hint("confirm", core.Keys.Select),
	core.Hint("cancel", core.Keys.Back),
}

// newLocationForm is the post-install "confirm install location" form, shown after
// a single install whose resolved path differs from the entry's prior manifest path
// (a path-less or relocated entry). The user can confirm or correct the install dir
// (corrections move the files) and optionally record the path in the global list.
// inGlobal selects the global toggle's meaning: export the repo (default skip) when
// it isn't in the global list, or update its remembered path (default on) when it is.
func newLocationForm(selected addon.Addon, pick versionItem, instPath, instVersion string, inGlobal bool) *components.FormScreen {
	pathF := components.NewTextField("path", "Path:    ", "addons/<name>")
	pathF.SetValue(instPath)

	action := "Export to Global"
	if inGlobal {
		action = "Update Global Path"
	}
	globalF := components.NewToggleField("global", "Global:  ", []string{"Skip", action}, "|")
	if inGlobal {
		globalF.OnToggle(true) // default to performing the update for an existing entry
	}

	form := components.NewForm(components.FormOpts{
		Crumb: "Install location",
		Fields: []components.FormField{
			components.NewHeading("Confirm install location for " + selected.Name),
			components.NewSpacer(),
			pathF,
			components.NewSpacer(),
			globalF,
		},
		Focus: "path",
		Help:  locationFormHelp,
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			return commitLocation(sh, selected, pick, instPath, instVersion, f, globalF)
		},
		// Dismissing without confirming leaves the files at the resolved path but
		// pins nothing — log that so the entry's unpinned state isn't a surprise.
		OnCancel: func(sh *core.Shared) core.Action {
			return core.Seq(
				core.SetStatusAndLog(selected.Name+": path not pinned (files left at "+instPath+")"),
				core.ShowTab(appctx.TitleProject),
			)
		},
	})
	return form
}

// commitLocation validates and applies the location form: it relocates the installed
// files when the path was corrected, pins the project manifest entry, and optionally
// exports/updates the global entry, then refreshes both lists and shows Project.
func commitLocation(sh *core.Shared, selected addon.Addon, pick versionItem, instPath, instVersion string, f *components.FormScreen, globalF *components.ToggleField) core.Action {
	c := appctx.Of(sh)

	finalPath, ok := cleanProjectPath(f.Value("path"))
	if !ok {
		return core.Seq(
			core.SetStatusAndLog("invalid path — must be project-relative"),
			core.Async(f.Focus("path")),
		)
	}

	moved := finalPath != instPath
	if moved {
		if err := addon.Relocate(c.ProjectRoot, instPath, finalPath); err != nil {
			return core.Seq(
				core.SetStatusAndLog("error: "+err.Error()),
				core.Async(f.Focus("path")),
			)
		}
	}

	pinStatus := pinInstall(c.ManifestPath, selected, pick, finalPath, instVersion)

	// Log only what actually changed: a move and/or a global write each get a log
	// line; a quiet accept (no move, global skipped) keeps just a transient status.
	acts := []core.Action{core.PropagateAll(appctx.ProjectDirty{})}
	logged := false
	if moved {
		acts = append(acts, core.SetStatusAndLog("moved "+selected.Name+" → "+finalPath))
		logged = true
	}
	if globalF.Index() == globalDo {
		if err := applyGlobal(c, selected, finalPath); err != nil {
			acts = append(acts, core.SetStatusAndLog("global: "+err.Error()))
		} else {
			acts = append(acts, core.SetStatusAndLog(globalActionMsg(c, selected)))
			acts = append(acts, core.PropagateAll(appctx.GlobalDirty{}))
		}
		logged = true
	}
	if !logged {
		acts = append(acts, core.SetStatus(pinStatus))
	}
	acts = append(acts, core.ShowTab(appctx.TitleProject))
	return core.Seq(acts...)
}

// globalActionMsg describes the global write for the log: an update when the repo was
// already listed, an export otherwise. Computed from the cached list (pre-write).
func globalActionMsg(c *appctx.Ctx, selected addon.Addon) string {
	if inGlobal, _ := globalEntry(selected.URL, c.GlobalAddons); inGlobal {
		return "updated global path for " + selected.Name
	}
	return "exported " + selected.Name + " to global"
}

// applyGlobal records the install path in the global list: it adds a new url+path
// entry when the repo isn't there yet, or updates the existing entry's path when it
// is (matched by canonical repo id). The url is stripped to its canonical repo form.
func applyGlobal(c *appctx.Ctx, selected addon.Addon, path string) error {
	globalPath, err := addon.GlobalListPath()
	if err != nil {
		return err
	}
	if inGlobal, gName := globalEntry(selected.URL, c.GlobalAddons); inGlobal {
		return addon.UpdateEntry(globalPath, gName, "", path, "", "")
	}
	url := selected.URL
	if stripped, err := source.RepoURL(selected.URL); err == nil {
		url = stripped
	}
	return addon.AddEntry(globalPath, selected.Name, url, path)
}

// globalEntry reports whether url's repo is already in the global list and, if so,
// the key name of that entry (matched by canonical repo id, since names are labels).
func globalEntry(url string, globals []addon.Addon) (bool, string) {
	id, err := source.RepoID(url)
	if err != nil {
		return false, ""
	}
	for _, g := range globals {
		if gid, err := source.RepoID(g.URL); err == nil && gid == id {
			return true, g.Name
		}
	}
	return false, ""
}

// cleanProjectPath validates and normalizes a user-entered install path: it must be
// non-empty, relative, and not escape the project root.
func cleanProjectPath(raw string) (string, bool) {
	p := filepath.Clean(strings.TrimSpace(raw))
	if p == "" || p == "." || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
		return "", false
	}
	return p, true
}

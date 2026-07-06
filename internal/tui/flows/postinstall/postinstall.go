// Package postinstall is the shared "confirm install location" flow: after an install
// pins an addon to a derived path, it lets the user confirm or correct where it landed
// (a correction moves the files) and optionally record the path in the global list.
// It runs a queue of Targets one form at a time, so both the single-install (project
// tab) and the batch flows (actions tab) drive it the same way. It sits in the flows
// layer (core ← components ← appctx ← flows ← tabs), so tabs compose it without
// importing each other.
package postinstall

import (
	"fmt"
	"path/filepath"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
)

// Target is one installed addon awaiting a location confirmation. Path is the current
// installed location (prefilled, and the source of a relocate); URL strips to the
// canonical repo url for the global list. The entry is already pinned to Path on disk
// — the form only optionally relocates it and records it globally.
type Target struct {
	Name    string
	URL     string
	Path    string
	Version string
}

// global toggle options (index 0 = skip the global action, 1 = perform it).
const (
	globalSkip = iota
	globalDo
)

// skipAllKey skips the rest of a queued form sequence at once. A non-text binding so
// the path field still accepts typing.
const skipAllKey = "ctrl+s"

var skipAllBind = key.NewBinding(key.WithKeys(skipAllKey), key.WithHelp("ctrl+s", "skip rest"))

var formHelp = []key.Binding{
	core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
	core.Hint("global", core.Keys.Left, core.Keys.Right),
	core.Hint("confirm", core.Keys.Select),
	core.Hint("keep", core.Keys.Back),
	skipAllBind,
}

// New returns the location form for the first of targets (len(targets) must be >= 1).
// Confirm/keep advances to the next target, finishing on the Project tab when the
// queue empties; ctrl+s keeps every remaining target at once.
func New(sh *core.Shared, targets []Target) *components.FormScreen {
	t := targets[0]
	rest := targets[1:]
	inGlobal, _ := globalEntry(t.URL, appctx.Of(sh).GlobalAddons)

	pathF := components.NewTextField("path", "Path:    ", "addons/<name>")
	pathF.SetValue(t.Path)

	action := "Export to Global"
	if inGlobal {
		action = "Update Global Path"
	}
	globalF := components.NewToggleField("global", "Global:  ", []string{"Skip", action}, "|")
	if inGlobal {
		globalF.OnToggle(true) // default to performing the update for an existing entry
	}

	heading := "Confirm install location for " + t.Name
	if len(targets) > 1 {
		heading += fmt.Sprintf("   (%d remaining)", len(targets))
	}

	return components.NewForm(components.FormOpts{
		Crumb: "Install location",
		Fields: []components.FormField{
			components.NewHeading(heading),
			components.NewSpacer(),
			pathF,
			components.NewSpacer(),
			globalF,
		},
		Focus: "path",
		Help:  formHelp,
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			return commit(sh, t, rest, f, globalF)
		},
		// Dismiss keeps the already-pinned path; log it and move on.
		OnCancel: func(sh *core.Shared) core.Action {
			return advance(sh, rest, core.SetStatusAndLog(t.Name+": kept at "+t.Path))
		},
		OnKey: func(sh *core.Shared, k string) (core.Action, bool) {
			if k == skipAllKey {
				return skipAll(rest), true
			}
			return core.Action{}, false
		},
	})
}

// commit validates and applies one target's form: relocate the files when the path was
// corrected (re-pinning the new path), optionally export/update the global entry, then
// advance to the next target.
func commit(sh *core.Shared, t Target, rest []Target, f *components.FormScreen, globalF *components.ToggleField) core.Action {
	c := appctx.Of(sh)

	finalPath, ok := cleanProjectPath(f.Value("path"))
	if !ok {
		return core.Seq(
			core.SetStatusAndLog("invalid path — must be project-relative"),
			core.Async(f.Focus("path")),
		)
	}

	moved := finalPath != t.Path
	if moved {
		if err := addon.Relocate(c.ProjectRoot, t.Path, finalPath); err != nil {
			return core.SeqErr(err, core.Async(f.Focus("path")))
		}
		_ = addon.UpdateEntry(c.ManifestPath, t.Name, "", finalPath, "", "")
	}

	// Log only what changed: a move and/or a global write each get a log line; a quiet
	// accept (no move, global skipped) keeps just a transient status.
	var logs []core.Action
	if moved {
		logs = append(logs, core.SetStatusAndLog("moved "+t.Name+" → "+finalPath))
	}
	if globalF.Index() == globalDo {
		if err := applyGlobal(c, t, finalPath); err != nil {
			logs = append(logs, core.SetStatusAndLog("global: "+err.Error()))
		} else {
			logs = append(logs, core.SetStatusAndLog(globalActionMsg(c, t)))
			logs = append(logs, core.PropagateAll(appctx.GlobalDirty{}))
		}
	}
	if len(logs) == 0 {
		logs = append(logs, core.SetStatus("kept "+t.Name+" at "+finalPath))
	}
	return advance(sh, rest, logs...)
}

// advance moves to the next target's form, or finishes on the Project tab when none
// remain. extra actions (status/log lines) run first.
func advance(sh *core.Shared, rest []Target, extra ...core.Action) core.Action {
	if len(rest) == 0 {
		return finish(extra...)
	}
	acts := append([]core.Action{}, extra...)
	acts = append(acts, core.Replace(New(sh, rest)))
	return core.Seq(acts...)
}

// finish ends the sequence: it reloads the Project list (a single broadcast for the
// whole batch) and shows it.
func finish(extra ...core.Action) core.Action {
	acts := append([]core.Action{}, extra...)
	acts = append(acts, core.PropagateAll(appctx.ProjectDirty{}), core.ShowTab(appctx.TitleProject))
	return core.Seq(acts...)
}

// skipAll keeps the current target and every remaining one at their installed paths,
// then finishes.
func skipAll(rest []Target) core.Action {
	n := len(rest) + 1
	return finish(core.SetStatusAndLog(fmt.Sprintf("kept %d addon(s) at their installed paths", n)))
}

// applyGlobal records the install path in the global list: it updates the existing
// entry's path when the repo is already listed (matched by canonical repo id), else
// adds a new url+path entry with the url stripped to its canonical repo form.
func applyGlobal(c *appctx.Ctx, t Target, path string) error {
	globalPath, err := addon.GlobalListPath()
	if err != nil {
		return err
	}
	if inGlobal, gName := globalEntry(t.URL, c.GlobalAddons); inGlobal {
		return addon.UpdateEntry(globalPath, gName, "", path, "", "")
	}
	url := t.URL
	if stripped, err := source.RepoURL(t.URL); err == nil {
		url = stripped
	}
	return addon.AddEntry(globalPath, t.Name, url, path)
}

// globalActionMsg describes the global write for the log: an update when the repo was
// already listed, an export otherwise. Read from the cached list (pre-write).
func globalActionMsg(c *appctx.Ctx, t Target) string {
	if inGlobal, _ := globalEntry(t.URL, c.GlobalAddons); inGlobal {
		return "updated global path for " + t.Name
	}
	return "exported " + t.Name + " to global"
}

// globalEntry reports whether url's repo is already in the global list and, if so, the
// key name of that entry (matched by canonical repo id, since names are labels).
func globalEntry(url string, globals []addon.Addon) (bool, string) {
	if e, ok := addon.FindByRepo(globals, url); ok {
		return true, e.Name
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

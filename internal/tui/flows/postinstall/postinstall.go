// Package postinstall is the "confirm install location" flow: after an install lands at a
// derived path, the user confirms or corrects it (moving the files) and may record it in
// the global list. It walks a queue of targets, for single and batch installs alike.
package postinstall

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
)

// Target is one installed addon awaiting confirmation. Path is where it is (already pinned);
// URL is reduced to the repo url for the global list.
type Target struct {
	// Name is the manifest key every write here addresses; Display is the entry's own
	// name, shown instead wherever a human reads it.
	Name    string
	Display string
	URL     string
	Path    string
	Version string
}

// label is the target's human-facing name: its own when recorded, else its key.
func (t Target) label() string {
	return addon.Addon{Name: t.Name, Display: t.Display}.Label()
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

// New returns the form for the first target (at least one required). Confirming advances;
// the Project tab shows when the queue empties; ctrl+s keeps all remaining.
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

	heading := "Confirm install location for " + t.label()
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
			return advance(sh, rest, core.SetStatusAndLog(t.label()+": kept at "+t.Path))
		},
		OnKey: func(sh *core.Shared, k string) (core.Action, bool) {
			if k == skipAllKey {
				return skipAll(rest), true
			}
			return core.Action{}, false
		},
	})
}

// commit applies one target: move the files if the path changed (re-pinning), optionally
// update the global entry, then advance.
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
		// The files moved; a failed re-pin would desync the manifest from disk, so
		// surface it and keep the form open rather than advancing as if it landed.
		if err := addon.UpdateEntry(c.ManifestPath, t.Name, "", finalPath, "", ""); err != nil {
			return core.SeqErr(err, core.Async(f.Focus("path")))
		}
	}

	// Log only what changed: a move and/or a global write each get a log line; a quiet
	// accept (no move, global skipped) keeps just a transient status.
	var logs []core.Action
	if moved {
		logs = append(logs, core.SetStatusAndLog("moved "+t.label()+" → "+finalPath))
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
		logs = append(logs, core.SetStatus("kept "+t.label()+" at "+finalPath))
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

// applyGlobal records the path in the global list, updating the repo's entry or adding one
// with its canonical url.
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
	// AddEntryFull so the addon's own name is exported alongside the url and path.
	return addon.AddEntryFull(globalPath, addon.Addon{Name: t.Name, Display: t.Display, URL: url, Path: path})
}

// globalActionMsg describes the global write for the log: an update when the repo was
// already listed, an export otherwise. Read from the cached list (pre-write).
func globalActionMsg(c *appctx.Ctx, t Target) string {
	if inGlobal, _ := globalEntry(t.URL, c.GlobalAddons); inGlobal {
		return "updated global path for " + t.label()
	}
	return "exported " + t.label() + " to global"
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

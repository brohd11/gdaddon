package newplugin

// This file holds the pieces the three add flows (Add Plugin, Store Asset, Track
// Installed) share: the url/name/path form skeleton, its submit pipeline, the
// Project/Global confirm dialog, and the add commit. Each flow keeps its own file for
// what genuinely differs — the store flow preserves the canonical store url and pins
// the release identity as the tag, the track flow upserts instead of adding — and
// plugs those deltas into the helpers here, so the shared shape is written once and
// the differences stay visible at the call site.

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
)

// ---------- form ----------

// formSpec is everything the three url/name/path forms differ on; the shared skeleton
// — heading, the three text fields, the spacer layout, and the
// field/toggle/next/cancel help row — stays in newAddonForm.
type formSpec struct {
	crumb          string                          // router breadcrumb
	heading        string                          // form heading line
	urlPlaceholder string                          // github repo vs canonical store url
	focus          string                          // key of the initially focused field
	toggleLabel    string                          // help label for the Left/Right toggle ("target" or "kind")
	values         map[string]string               // prefilled field values (empty values are skipped)
	tail           []components.FormField          // trailing fields: the toggle plus an optional note
	onSubmit       func(*core.Shared, *components.FormScreen) core.Action
}

// newAddonForm builds the url/name/path form every flow opens on, appending the
// flow's trailing fields (a target or kind toggle, an optional note) after the shared
// text fields and pre-filling any values.
func newAddonForm(spec formSpec) *components.FormScreen {
	fields := []components.FormField{
		components.NewHeading(spec.heading),
		components.NewSpacer(),
		components.NewTextField("url", "URL:     ", spec.urlPlaceholder),
		components.NewTextField("name", "Name:    ", "(optional — derived from url)"),
		components.NewTextField("path", "Path:    ", "(optional — derived on install)"),
		components.NewSpacer(),
	}
	fields = append(fields, spec.tail...)
	form := components.NewForm(components.FormOpts{
		Crumb:  spec.crumb,
		Fields: fields,
		Focus:  spec.focus,
		Help: []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint(spec.toggleLabel, core.Keys.Left, core.Keys.Right),
			core.Hint("next", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		},
		OnSubmit: spec.onSubmit,
	})
	for k, v := range spec.values {
		if v != "" {
			form.SetValue(k, v)
		}
	}
	return form
}

// submitAddonForm is the shared OnSubmit pipeline: trim the url and refocus its field
// when it is empty, normalize it (a nil normalize keeps it as typed — the store flow
// preserves the canonical store url rather than mangling it into a .git url), derive
// a blank name from the final url, and hand the values to the flow's confirm push.
func submitAddonForm(f *components.FormScreen, normalize func(string) string, next func(name, url, path string) core.Action) core.Action {
	url := strings.TrimSpace(f.Value("url"))
	if url == "" {
		return core.Async(f.Focus("url"))
	}
	if normalize != nil {
		url = normalize(url)
	}
	name := strings.TrimSpace(f.Value("name"))
	if name == "" {
		name = addon.DeriveName(url)
	}
	return next(name, url, strings.TrimSpace(f.Value("path")))
}

// ---------- confirm ----------

// newTargetConfirm builds the confirm dialog the plugin and store flows share: the
// rendered body plus a Project/Global toggle the Left/Right keys flip before OnYes
// commits. addTarget seeds the toggle from the form's value; body and onYes receive
// the (possibly flipped) target.
func newTargetConfirm(addTarget int, body func(sh *core.Shared, target int) string, onYes func(sh *core.Shared, target int) core.Action) *components.DialogScreen {
	target := addTarget // local copy the toggle mutates
	return &components.DialogScreen{
		Render: func(sh *core.Shared) string { return sh.Box(body(sh, target)) },
		OnKey: func(sh *core.Shared, k string) core.Action {
			if core.MatchKey(k, core.Keys.Left) || core.MatchKey(k, core.Keys.Right) {
				target = otherTarget(target)
			}
			return core.Action{}
		},
		OnYes: func(sh *core.Shared) core.Action { return onYes(sh, target) },
		Help:  newPluginConfirmHelp,
	}
}

// confirmBody renders the field block every confirm screen shows: the name, an
// optional version line, the hard-wrapped url indented under its label, and the path
// (defaulted when blank). title is the flow's heading; extra is whatever the flow
// appends after the path (the Project/Global toggle line, the track kind line, or
// nothing).
func confirmBody(sh *core.Shared, title, name, version, url, path, extra string) string {
	urlBlock := core.IndentLines(core.HardWrap(url, sh.ConfirmWidth()-4), "    ")
	if path == "" {
		path = "(derived on install)"
	}
	body := fmt.Sprintf("%s\n\n  name:     %s", title, name)
	if version != "" {
		body += "\n  version:  " + version
	}
	body += fmt.Sprintf("\n  url:\n%s\n  path:     %s", urlBlock, path)
	return body + extra
}

// addToLine is the trailing Project/Global toggle line of the plugin and store
// confirm bodies.
func addToLine(target int) string {
	return "\n\n  add to:   " + components.RenderToggle(targetOptions, target, "")
}

// ---------- commit ----------

// commitAdd is the shared commit pipeline of the plugin and store flows: a global add
// appends to the global list and shows the rebuilt Global tab; a project add runs
// addEntry against the project manifest (the flows differ only in which manifest
// operation that is) and shows the Browse tab. Both unwind to the root and flag the
// affected list dirty.
func commitAdd(sh *core.Shared, name, url, path string, addTarget int, addEntry func(manifestPath string) error) core.Action {
	if addTarget == targetGlobal {
		globalPath, err := addon.GlobalListPath()
		if err == nil {
			err = addon.AddEntry(globalPath, name, url, path)
		}
		if err != nil {
			return core.SeqErr(err, core.ResetToRoot())
		}
		// Show the Global tab rebuilt with the new entry (parallel to a project add
		// switching to Browse).
		return core.Seq(
			core.SetStatus(fmt.Sprintf("added %s to global list", name)),
			core.PropagateAll(appctx.GlobalDirty{}),
			core.ShowTab(appctx.TitleGlobal),
		)
	}

	if err := addEntry(appctx.Of(sh).ManifestPath); err != nil {
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

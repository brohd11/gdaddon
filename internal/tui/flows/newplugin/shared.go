package newplugin

// Pieces shared by the three add flows (Add Plugin, Store Asset, Track Installed): the form
// skeleton, its submit pipeline, the Project/Global confirm and the add commit. Each flow's
// file holds only what differs.

import (
	"fmt"
	"strings"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
)

// ---------- form ----------

// formSpec is what the url/name/path forms differ on; newAddonForm builds the rest.
type formSpec struct {
	crumb          string                 // router breadcrumb
	heading        string                 // form heading line
	urlPlaceholder string                 // github repo vs canonical store url
	focus          string                 // key of the initially focused field
	toggleLabel    string                 // help label for the Left/Right toggle ("target" or "kind")
	values         map[string]string      // prefilled field values (empty values are skipped)
	tail           []components.FormField // trailing fields: the toggle plus an optional note
	onSubmit       func(*core.Shared, *components.FormScreen) core.Action
}

// newAddonForm builds the url/name/path form, appends the flow's extra fields and prefills
// values.
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

// submitAddonForm is the shared OnSubmit: require a url, normalize it (nil keeps it, as the
// store flow needs), default a blank name from it, and push the flow's confirm. A typed
// name becomes the key, which lets you track two checkouts of one repo.
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
		name = addon.EntryKey(url)
	}
	return next(name, url, strings.TrimSpace(f.Value("path")))
}

// ---------- confirm ----------

// newTargetConfirm is the plugin and store flows' confirm: the body plus a Project/Global
// toggle (Left/Right) passed to onYes.
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

// confirmBody renders the shared confirm fields: name, optional version, wrapped url and
// path (defaulted when blank), then extra.
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

// commitAdd writes the entry: to the global list (showing Global), or via addEntry to the
// project manifest (showing the project list). Both unwind to the root and mark the list
// dirty.
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

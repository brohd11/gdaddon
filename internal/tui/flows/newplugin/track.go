package newplugin

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/store"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
)

var trackConfirmHelp = []key.Binding{
	core.Hint("add", core.Keys.Select),
	core.Hint("back", core.Keys.Back),
}

// NewFromInstall builds the form for tracking an installed plugin found by Scan: name and
// path from disk, a suggested url (focused, to confirm), and for a git checkout its kind
// and branch (recorded as tag). Submitting upserts the entry, filling in a matching
// path-less entry or adding one.
func NewFromInstall(path, name, version, suggestedURL string, kind addon.Kind, branch string) *components.FormScreen {
	kindF := components.NewToggleField("kind", "Kind:    ", addon.KindOptions, "|")
	kindF.SetIndex(addon.KindIndex(kind))

	return newAddonForm(formSpec{
		crumb:          "Track Plugin",
		heading:        "Track installed plugin",
		urlPlaceholder: "https://github.com/owner/repo",
		focus:          "url",
		toggleLabel:    "kind",
		values:         map[string]string{"url": suggestedURL, "name": name, "path": path},
		tail: []components.FormField{
			kindF,
			components.NewNote("  installed " + versionLabel(version)),
		},
		onSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			return submitAddonForm(f, normalizeTrackURL, func(name, url, path string) core.Action {
				return core.Push(newTrackConfirm(name, url, path, version, addon.ParseKind(kindF.Value()), branch))
			})
		},
	})
}

// normalizeTrackURL normalizes a git url but leaves a store url as typed — store urls
// are canonical and must never gain a .git suffix.
func normalizeTrackURL(url string) string {
	if store.IsStoreURL(url) {
		return url
	}
	return addon.NormalizeRepoURL(url)
}

// newTrackConfirm is the track flow's confirm, with no target toggle (always the project).
func newTrackConfirm(name, url, path, version string, kind addon.Kind, branch string) *components.DialogScreen {
	return &components.DialogScreen{
		Render: func(sh *core.Shared) string {
			return sh.Box(trackConfirmBody(sh, name, url, path, version, kind, branch))
		},
		OnYes: func(sh *core.Shared) core.Action { return commitTrack(sh, name, url, path, version, kind, branch) },
		Help:  trackConfirmHelp,
	}
}

func trackConfirmBody(sh *core.Shared, name, url, path, version string, kind addon.Kind, branch string) string {
	extra := ""
	if kind != addon.KindPackage {
		extra = "\n  kind:     " + string(kind)
		if branch != "" {
			extra += " (branch " + branch + ")"
		}
	}
	return confirmBody(sh, "Track plugin", name, versionLabel(version), url, path, extra)
}

// commitTrack upserts the plugin into the project manifest (matched by repo identity), with
// its kind and, for checkouts, the branch as tag.
func commitTrack(sh *core.Shared, name, url, path, version string, kind addon.Kind, branch string) core.Action {
	manifestPath := appctx.Of(sh).ManifestPath
	a := addon.Addon{Name: name, URL: url, Path: path, Version: version, Kind: kind}
	if a.IsGitWorkdir() {
		a.Tag = branch
	}
	if err := addon.UpsertEntry(manifestPath, a); err != nil {
		return core.Seq(
			core.SetStatus("error: "+err.Error()),
			core.PopTo(),
		)
	}
	return core.Seq(
		core.SetStatus("tracking "+name),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.PopTo(),
	)
}

func versionLabel(version string) string {
	if version == "" {
		return "(version unknown)"
	}
	return "v" + version
}

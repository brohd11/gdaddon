package newplugin

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/store"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
)

var trackConfirmHelp = []key.Binding{
	core.Hint("add", core.Keys.Select),
	core.Hint("back", core.Keys.Back),
}

// NewFromInstall builds the form for tracking an already-installed plugin found by
// the Scan action: name and path are prefilled from disk and url is prefilled with a
// suggestion (a git checkout's origin remote, a `source=` cfg key, or a matching
// pathless manifest entry), with focus on the url field so the user confirms it. The
// kind picker is pre-set when the folder is a git checkout — clone for a standalone
// repo, submodule for a parent-managed one — and branch is its checked-out branch,
// recorded as the entry's tag. On submit it upserts the project entry — backfilling
// path/version on a matching pathless entry (the cogito case) or adding a new one —
// so a bundled/sideloaded plugin (or submodule) starts being tracked.
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

// newTrackConfirm is the track flow's own confirm: unlike the plugin/store confirms
// there is no Project/Global toggle (tracking always targets the project manifest),
// so it only renders the body and commits on yes.
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

// commitTrack upserts the installed plugin into the project manifest: UpsertEntry
// matches by repo identity, so it backfills path+version on an existing pathless
// entry or appends a new one, and sets the kind from the Addon. For a git checkout
// (clone or submodule) it records the branch as the entry's tag (what cloneInstall
// clones, and what a submodule entry displays).
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

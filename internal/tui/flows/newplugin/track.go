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
	urlF := components.NewTextField("url", "URL:     ", "https://github.com/owner/repo")
	nameF := components.NewTextField("name", "Name:    ", "(optional — derived from url)")
	pathF := components.NewTextField("path", "Path:    ", "(optional — derived on install)")
	kindF := components.NewToggleField("kind", "Kind:    ", addon.KindOptions, "|")

	urlF.SetValue(suggestedURL)
	nameF.SetValue(name)
	pathF.SetValue(path)
	kindF.SetIndex(addon.KindIndex(kind))

	return components.NewForm(components.FormOpts{
		Crumb: "Track Plugin",
		Fields: []components.FormField{
			components.NewHeading("Track installed plugin"),
			components.NewSpacer(),
			urlF, nameF, pathF,
			components.NewSpacer(),
			kindF,
			components.NewNote("  installed " + versionLabel(version)),
		},
		Focus: "url",
		Help: []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint("kind", core.Keys.Left, core.Keys.Right),
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
			return core.Push(newTrackConfirm(name, url, path, version, addon.ParseKind(kindF.Value()), branch))
		},
	})
}

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
	urlBlock := core.IndentLines(core.HardWrap(url, sh.ConfirmWidth()-4), "    ")
	if path == "" {
		path = "(derived on install)"
	}
	body := fmt.Sprintf(
		"Track plugin\n\n  name:     %s\n  version:  %s\n  url:\n%s\n  path:     %s",
		name, versionLabel(version), urlBlock, path)
	if kind != addon.KindPackage {
		kindLine := "\n  kind:     " + string(kind)
		if branch != "" {
			kindLine += " (branch " + branch + ")"
		}
		body += kindLine
	}
	return body
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

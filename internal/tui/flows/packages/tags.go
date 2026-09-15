package packages

import (
	"context"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/source"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type tagsMsg struct {
	tags []source.Release
	err  error
}

func newTagsLoading(repoID, repoURL string, opts BrowseOpts, archived archivedSet) *components.LoadingScreen {
	run := func(ctx context.Context) tea.Cmd {
		return func() tea.Msg {
			tags, err := source.Tags(ctx, repoURL)
			return tagsMsg{tags: tags, err: err}
		}
	}
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		m, ok := msg.(tagsMsg)
		if !ok {
			return core.Action{}
		}
		if m.err != nil {
			return core.SeqErr(m.err, core.Pop())
		}
		var items []list.Item
		for _, tag := range m.tags {
			items = append(items, components.Item{
				Name: tag.Tag,
				Desc: "release assets or source ZIP",
				Pick: func(sh *core.Shared) core.Action {
					return core.Push(newTagLoading(repoID, repoURL, tag.Tag, opts, archived))
				},
			})
		}
		items = components.EnsurePlaceholder(items, "(no tags found)", "")
		return core.Replace(components.NewPicker(items, components.PickerOpts{Crumb: "Tags", Title: repoID}))
	}
	return components.NewLoadingScreen(repoID, "fetching tags…", run, onResult)
}

type tagMsg struct {
	release source.Release
	err     error
}

// A tag may have a published build: resolve it before offering the source ZIP.
func newTagLoading(repoID, repoURL, tag string, opts BrowseOpts, archived archivedSet) *components.LoadingScreen {
	run := func(ctx context.Context) tea.Cmd {
		return func() tea.Msg {
			rel, err := source.ResolveTag(ctx, repoURL, tag)
			return tagMsg{release: rel, err: err}
		}
	}
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		m, ok := msg.(tagMsg)
		if !ok {
			return core.Action{}
		}
		if m.err != nil {
			return core.SeqErr(m.err, core.Pop())
		}
		if len(m.release.Assets) == 1 {
			return core.Replace(opts.Endpoint(releaseSelection(repoID, m.release, m.release.Assets[0], archived)))
		}
		return core.Replace(newAssetPicker(repoID, m.release, opts, archived))
	}
	return components.NewLoadingScreen(tag, "resolving tag…", run, onResult)
}

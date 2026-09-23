package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/brohd11/gdaddon/internal/config"
	searchpkg "github.com/brohd11/gdaddon/internal/search"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/newplugin"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// ---------- result/detail messages ----------

// searchResultMsg / detailMsg carry an asset-source fetch back to a loading
// screen's onResult closure (the generic LoadingScreen never names them).
type searchResultMsg struct {
	res *searchpkg.Page
	err error
}

type detailMsg struct {
	detail *searchpkg.Detail
	err    error
}

// ---------- query screen ----------

// newQueryScreen builds the search form: a Source row (Enter opens a menu), the query field
// and a note with the Godot version filter. The form variable is declared first because
// the Source row's handler anchors the menu to it.
func newQueryScreen(src searchpkg.Source, godotVer, lastQuery string) *components.FormScreen {
	cur := src
	var form *components.FormScreen
	source := components.NewPickField("source", "Source:  ",
		func() string { return cur.Name() },
		func(sh *core.Shared) (core.Action, bool) { return core.Push(sourceMenu(sh, form, &cur)), true })
	query := components.NewTextField("query", "Query:   ", "search terms (e.g. dialogue)")
	query.SetValue(lastQuery)

	form = components.NewForm(components.FormOpts{
		Crumb: "Search",
		Fields: []components.FormField{
			components.NewHeading("Search assets"),
			components.NewSpacer(),
			source,
			query,
			components.NewSpacer(),
			components.NewNote("  filtering by Godot " + godotVer),
		},
		Focus: "query",
		Help: []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint("go", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		},
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			q := strings.TrimSpace(f.Value("query"))
			if q == "" {
				return core.Action{}
			}
			appctx.Of(sh).LastSearchQuery = q
			_ = config.SaveLastSource(cur.Name()) // best-effort, like SaveTheme
			return core.Push(newSearchLoading(cur, q, godotVer, 0))
		},
	})
	return form
}

// ---------- source menu ----------

// sourceMenu opens the sources as a dropdown under the Source row; the choice is written
// through dst and the menu pops itself.
func sourceMenu(sh *core.Shared, form *components.FormScreen, dst *searchpkg.Source) *components.MenuScreen {
	srcs := searchpkg.Sources()
	items := make([]components.MenuItem, 0, len(srcs))
	cursor := 0
	for i, src := range srcs {
		src := src
		hint := ""
		if src.Name() == (*dst).Name() {
			hint, cursor = "✓", i
		}
		items = append(items, components.MenuItem{
			Label: src.Name(),
			Hint:  hint,
			Pick:  func(sh *core.Shared) core.Action { *dst = src; return core.Pop() },
		})
	}
	// The key is the literal newQueryScreen registers, so the miss is unreachable; the
	// fallback opens at the body's top-left rather than at cell (0,0) under the header.
	anchor, ok := form.FieldAnchor(sh, "source")
	if !ok {
		anchor = components.AnchorBelow(0, sh.BodyY())
	}
	m := components.NewMenu(components.MenuOpts{Items: items, Anchor: anchor})
	m.Select(cursor)
	return m
}

// ---------- search loading + results ----------

func newSearchLoading(src searchpkg.Source, query, godotVer string, page int) *components.LoadingScreen {
	cmd := func(ctx context.Context) tea.Cmd {
		return func() tea.Msg {
			res, err := src.Search(ctx, query, godotVer, page)
			return searchResultMsg{res: res, err: err}
		}
	}
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		m, ok := msg.(searchResultMsg)
		if !ok {
			return core.Action{}
		}
		if m.err != nil {
			return core.SeqErr(m.err, core.Pop())
		}
		if len(m.res.Results) == 0 {
			return core.Seq(
				core.SetStatusAndLog(fmt.Sprintf("no results for %q", query)),
				core.Pop(),
			)
		}
		return core.Replace(newResultsPicker(src, query, godotVer, m.res))
	}
	return components.NewLoadingScreen(src.Name(), "searching…", cmd, onResult)
}

// newResultsPicker shows one page of results. Each row hands off to the asset
// detail fetch; PageNext/PagePrev page within the bounds reported by the source.
func newResultsPicker(src searchpkg.Source, query, godotVer string, res *searchpkg.Page) *components.PickerScreen {
	items := make([]list.Item, 0, len(res.Results))
	for _, r := range res.Results {
		r := r
		items = append(items, components.Item{
			Name:   r.Title,
			Desc:   resultDesc(r),
			Filter: r.Title + " " + r.Author,
			Pick:   func(sh *core.Shared) core.Action { return core.Push(newDetailLoading(src, r.ID)) },
		})
	}
	title := fmt.Sprintf("%s · page %d/%d · %d results", src.Name(), res.Page+1, res.Pages, res.TotalItems)

	onKey := func(sh *core.Shared, k string, _ list.Item) (core.Action, bool) {
		switch {
		case core.MatchKey(k, core.Keys.PageNext):
			if res.Page+1 < res.Pages {
				return core.Replace(newSearchLoading(src, query, godotVer, res.Page+1)), true
			}
			return core.Action{}, true
		case core.MatchKey(k, core.Keys.PagePrev):
			if res.Page > 0 {
				return core.Replace(newSearchLoading(src, query, godotVer, res.Page-1)), true
			}
			return core.Action{}, true
		}
		return core.Action{}, false
	}
	help := []key.Binding{core.Hint("results", core.Keys.PageNext, core.Keys.PagePrev)}
	return components.NewPicker(items, components.PickerOpts{Crumb: "Results", Title: title, OnKey: onKey, Help: help})
}

func resultDesc(r searchpkg.Summary) string {
	parts := make([]string, 0, 4)
	if r.Author != "" {
		parts = append(parts, r.Author)
	}
	if r.Category != "" {
		parts = append(parts, r.Category)
	}
	if r.Cost != "" {
		parts = append(parts, r.Cost)
	}
	if r.GodotVersion != "" {
		parts = append(parts, "godot "+r.GodotVersion)
	}
	return strings.Join(parts, " · ")
}

// ---------- asset detail → New Plugin handoff ----------

// newDetailLoading fetches the asset's detail (for its urls) and opens the install menu.
func newDetailLoading(src searchpkg.Source, id string) *components.LoadingScreen {
	cmd := func(ctx context.Context) tea.Cmd {
		return func() tea.Msg {
			d, err := src.Detail(ctx, id)
			return detailMsg{detail: d, err: err}
		}
	}
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		m, ok := msg.(detailMsg)
		if !ok {
			return core.Action{}
		}
		if m.err != nil {
			return core.SeqErr(m.err, core.Pop())
		}
		return newInstallMenu(src, m.detail)
	}
	return components.NewLoadingScreen("Asset", "fetching asset…", cmd, onResult)
}

// newInstallMenu offers "Add repository" when there is a repo url, and "Add store asset"
// for a store source with a release; with neither, it reports and pops.
func newInstallMenu(src searchpkg.Source, d *searchpkg.Detail) core.Action {
	var items []list.Item
	if d.BrowseURL != "" {
		// Add repository handles any repo URL (not just GitHub); newplugin
		// normalizes it and resolves versions via the source resolver.
		items = append(items, components.Item{
			Name: "Add repository",
			Desc: d.BrowseURL,
			Pick: func(sh *core.Shared) core.Action { return core.Replace(newplugin.NewWithURL(d.BrowseURL)) },
		})
	}
	if storeSrc, ok := src.(searchpkg.AssetURLer); ok && d.DownloadURL != "" {
		url := storeSrc.AssetURL(d.ID)
		version := d.VersionString
		items = append(items, components.Item{
			Name: "Add store asset",
			Desc: "release " + d.VersionString,
			Pick: func(sh *core.Shared) core.Action { return core.Replace(newplugin.NewStoreForm(url, version)) },
		})
	}
	if len(items) == 0 {
		return core.Seq(
			core.SetStatusAndLog("asset has nothing installable (no repository or release)"),
			core.Pop(),
		)
	}

	title := d.Title
	if d.VersionString != "" {
		title += " · v" + d.VersionString
	}
	return core.Replace(components.NewPicker(items, components.PickerOpts{Crumb: "Install", Title: title}))
}

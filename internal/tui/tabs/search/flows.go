package search

import (
	"context"
	"fmt"
	"strings"

	searchpkg "gdaddon/internal/search"
	"gdaddon/internal/tui/flows/newplugin"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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

// newQueryScreen builds the search entry form (a generic components.FormScreen): a
// Source row whose Enter opens the source sub-picker (a PickField/Activator), the
// query text field, and a muted note showing the Godot version filter. The chosen
// source is held in a captured variable that the sub-picker mutates and the
// PickField/OnSubmit read back.
func newQueryScreen(src searchpkg.Source, godotVer string) *components.FormScreen {
	cur := src
	source := components.NewPickField("source", "Source:  ",
		func() string { return cur.Name() },
		func(sh *core.Shared) (tea.Msg, tea.Cmd, bool) { return core.Push(newSourcePicker(&cur)), nil, true })
	query := components.NewTextField("query", "Query:   ", "search terms (e.g. dialogue)")

	return components.NewForm(components.FormOpts{
		Crumb: core.RenderTitleBar("Search"),
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
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) (tea.Msg, tea.Cmd) {
			q := strings.TrimSpace(f.Value("query"))
			if q == "" {
				return nil, nil
			}
			return core.Push(newSearchLoading(cur, q, godotVer, 0)), nil
		},
	})
}

// ---------- source picker ----------

// newSourcePicker lists the registered asset sources; selecting one writes it back
// through dst and pops to the query form. With a single source today it's a one-row
// list, but the threading is already source-agnostic.
func newSourcePicker(dst *searchpkg.Source) *components.PickerScreen {
	srcs := searchpkg.Sources()
	items := make([]list.Item, 0, len(srcs))
	for _, src := range srcs {
		src := src
		items = append(items, components.Item{
			Name: src.Name(),
			Pick: func(sh *core.Shared) (tea.Msg, tea.Cmd) { *dst = src; return core.Pop(), nil },
		})
	}
	return components.NewPicker(items, components.PickerOpts{Title: "Select source"})
}

// ---------- search loading + results ----------

func newSearchLoading(src searchpkg.Source, query, godotVer string, page int) *components.LoadingScreen {
	cmd := func() tea.Msg {
		res, err := src.Search(context.Background(), query, godotVer, page)
		return searchResultMsg{res: res, err: err}
	}
	onResult := func(sh *core.Shared, msg tea.Msg) (tea.Msg, tea.Cmd) {
		m, ok := msg.(searchResultMsg)
		if !ok {
			return nil, nil
		}
		if m.err != nil {
			sh.SetStatus("error: " + m.err.Error())
			return core.Pop(), nil
		}
		if len(m.res.Results) == 0 {
			sh.SetStatus(fmt.Sprintf("no results for %q", query))
			return core.Pop(), nil
		}
		return core.Replace(newResultsPicker(src, query, godotVer, m.res)), nil
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
			Pick:   func(sh *core.Shared) (tea.Msg, tea.Cmd) { return core.Push(newDetailLoading(src, r.ID)), nil },
		})
	}
	title := fmt.Sprintf("%s · page %d/%d · %d results", src.Name(), res.Page+1, res.Pages, res.TotalItems)

	onKey := func(sh *core.Shared, k string, _ list.Item) (tea.Msg, tea.Cmd, bool) {
		switch {
		case core.MatchKey(k, core.Keys.PageNext):
			if res.Page+1 < res.Pages {
				return core.Replace(newSearchLoading(src, query, godotVer, res.Page+1)), nil, true
			}
			return nil, nil, true
		case core.MatchKey(k, core.Keys.PagePrev):
			if res.Page > 0 {
				return core.Replace(newSearchLoading(src, query, godotVer, res.Page-1)), nil, true
			}
			return nil, nil, true
		}
		return nil, nil, false
	}
	help := []key.Binding{core.Hint("page", core.Keys.PageNext, core.Keys.PagePrev)}
	return components.NewPicker(items, components.PickerOpts{Title: title, OnKey: onKey, Help: help})
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

// newDetailLoading fetches the chosen asset's detail (the search list omits the
// repo URL) and hands its browse_url to the shared New Plugin form, prefilled.
func newDetailLoading(src searchpkg.Source, id string) *components.LoadingScreen {
	cmd := func() tea.Msg {
		d, err := src.Detail(context.Background(), id)
		return detailMsg{detail: d, err: err}
	}
	onResult := func(sh *core.Shared, msg tea.Msg) (tea.Msg, tea.Cmd) {
		m, ok := msg.(detailMsg)
		if !ok {
			return nil, nil
		}
		if m.err != nil {
			sh.SetStatus("error: " + m.err.Error())
			return core.Pop(), nil
		}
		if m.detail.BrowseURL == "" {
			sh.SetStatus("asset has no repository url")
			return core.Pop(), nil
		}
		return core.Replace(newplugin.NewWithURL(m.detail.BrowseURL)), nil
	}
	return components.NewLoadingScreen("Asset", "fetching asset…", cmd, onResult)
}

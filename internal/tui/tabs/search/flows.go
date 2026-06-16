package search

import (
	"context"
	"fmt"
	"strings"

	searchpkg "gdaddon/internal/search"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"
	"gdaddon/internal/tui/flows/newplugin"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// fields of the query screen: a source selector and the query text input.
const (
	fldSource = iota
	fldQuery
	fldCount
)

// queryScreen is the search entry point: pick a source (a list/submenu) and type
// a query. It captures keys (Filtering) so the global chrome shortcuts don't steal
// what's typed, and dispatches navigation via core.Keys; the typing-vs-navigation
// split (so letter-aliases like "c"/"e" reach the query box instead of triggering
// Back/Select) is handled centrally by components.QueryUpdate via the Typable interface.
type queryScreen struct {
	src      searchpkg.Source
	godotVer string
	input    textinput.Model
	focus    int
}

var _ core.Filterer = (*queryScreen)(nil)

func newQueryScreen(src searchpkg.Source, godotVer string) *queryScreen {
	ti := textinput.New()
	ti.Placeholder = "search terms (e.g. dialogue)"
	ti.Prompt = ""
	return &queryScreen{src: src, godotVer: godotVer, input: ti, focus: fldQuery}
}

func (s *queryScreen) Init(*core.Shared) tea.Cmd { return s.syncFocus() }

func (s *queryScreen) Filtering() bool { return true }

// Typable: the query box has focus only on the query field; QueryUpdate keeps
// typed characters there instead of letting them trigger navigation.
func (s *queryScreen) Typing() bool            { return s.focus == fldQuery }
func (s *queryScreen) Input() *textinput.Model { return &s.input }

func (s *queryScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	if cmd, ok := components.QueryUpdate(s, msg); ok {
		return s, cmd
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	k := key.String()
	switch {
	case core.MatchKey(k, core.Keys.Back):
		return s, core.Pop()
	case core.MatchKey(k, core.Keys.NextField):
		s.focus = (s.focus + 1) % fldCount
		return s, s.syncFocus()
	case core.MatchKey(k, core.Keys.PrevField):
		s.focus = (s.focus - 1 + fldCount) % fldCount
		return s, s.syncFocus()
	case core.MatchKey(k, core.Keys.Select):
		if s.focus == fldSource {
			return s, core.Push(newSourcePicker(s))
		}
		query := strings.TrimSpace(s.input.Value())
		if query == "" {
			return s, nil
		}
		return s, core.Push(newSearchLoading(s.src, query, s.godotVer, 0))
	}
	// Editing keys (backspace, cursor) fall through to the focused query box.
	if s.focus == fldQuery {
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s *queryScreen) syncFocus() tea.Cmd {
	if s.focus == fldQuery {
		return s.input.Focus()
	}
	s.input.Blur()
	return nil
}

func (s *queryScreen) View(sh *core.Shared) string {
	label := lipgloss.NewStyle().Foreground(core.MutedColor)
	marker := func(focused bool) string {
		if focused {
			return lipgloss.NewStyle().Foreground(core.FocusedColor).Render("▸ ")
		}
		return "  "
	}
	body := strings.Join([]string{
		"Search assets",
		"",
		marker(s.focus == fldSource) + label.Render("Source:  ") + s.src.Name(),
		marker(s.focus == fldQuery) + label.Render("Query:   ") + s.input.View(),
		"",
		label.Render("  filtering by Godot " + s.godotVer),
	}, "\n")
	return lipgloss.JoinVertical(lipgloss.Left,
		core.RenderTitleBar("Search"),
		sh.Box(body))
}

func (s *queryScreen) HelpView(sh *core.Shared) string {
	return sh.BindingHelp([]key.Binding{
		core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
		core.Hint("go", core.Keys.Select),
		core.Hint("cancel", core.Keys.Back),
	})
}

func (s *queryScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	w := sh.ConfirmWidth() - 12 // box room minus the label column
	if w < 10 {
		w = 10
	}
	s.input.Width = w
}

// ---------- source picker ----------

// newSourcePicker lists the registered asset sources; selecting one sets it on
// the query screen and pops back. With a single source today it's a one-row list,
// but the threading is already source-agnostic.
func newSourcePicker(qs *queryScreen) *components.PickerScreen {
	srcs := searchpkg.Sources()
	items := make([]list.Item, 0, len(srcs))
	for _, src := range srcs {
		src := src
		items = append(items, components.Item{
			Name: src.Name(),
			Pick: func(sh *core.Shared) tea.Cmd { qs.src = src; return core.Pop() },
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
	onResult := func(sh *core.Shared, msg tea.Msg) tea.Cmd {
		m, ok := msg.(searchResultMsg)
		if !ok {
			return nil
		}
		if m.err != nil {
			sh.StatusMsg = "error: " + m.err.Error()
			return core.Pop()
		}
		if len(m.res.Results) == 0 {
			sh.StatusMsg = fmt.Sprintf("no results for %q", query)
			return core.Pop()
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
			Pick:   func(sh *core.Shared) tea.Cmd { return core.Push(newDetailLoading(src, r.ID)) },
		})
	}
	title := fmt.Sprintf("%s · page %d/%d · %d results", src.Name(), res.Page+1, res.Pages, res.TotalItems)

	onKey := func(sh *core.Shared, k string, _ list.Item) (tea.Cmd, bool) {
		switch {
		case core.MatchKey(k, core.Keys.PageNext):
			if res.Page+1 < res.Pages {
				return core.Replace(newSearchLoading(src, query, godotVer, res.Page+1)), true
			}
			return nil, true
		case core.MatchKey(k, core.Keys.PagePrev):
			if res.Page > 0 {
				return core.Replace(newSearchLoading(src, query, godotVer, res.Page-1)), true
			}
			return nil, true
		}
		return nil, false
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
	onResult := func(sh *core.Shared, msg tea.Msg) tea.Cmd {
		m, ok := msg.(detailMsg)
		if !ok {
			return nil
		}
		if m.err != nil {
			sh.StatusMsg = "error: " + m.err.Error()
			return core.Pop()
		}
		if m.detail.BrowseURL == "" {
			sh.StatusMsg = "asset has no repository url"
			return core.Pop()
		}
		return core.Replace(newplugin.NewWithURL(m.detail.BrowseURL))
	}
	return components.NewLoadingScreen("Asset", "fetching asset…", cmd, onResult)
}

package app

import (
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"
	"github.com/brohd11/gitstack/repoui"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

const listTitle = "Repos"

// ReposScreen is repoview's single root screen — the scanned repo list. Enter opens the
// highlighted repo's git submenu; V the all-repos menu; f a concurrent fetch-all; a the Actions
// menu. It mirrors gdaddon's Project screen minus all the manifest/install machinery: the whole
// refresh story is "re-scan the directory".
type ReposScreen struct {
	list list.Model
	// fetching guards against a second f fanning out a duplicate set of fetches while the
	// first is still running; cleared when its fetchDone arrives.
	fetching bool
}

var _ core.Filterer = (*ReposScreen)(nil)
var _ core.Receiver = (*ReposScreen)(nil)
var _ core.Crumber = (*ReposScreen)(nil)

func NewReposScreen(sh *core.Shared) *ReposScreen {
	l := core.NewSelectList(repoListItems(sh), listTitle, keys.Fetch, keys.GitAll, keys.Actions)
	return &ReposScreen{list: l}
}

func (s *ReposScreen) Init(*core.Shared) tea.Cmd       { return nil }
func (s *ReposScreen) Filtering() bool                 { return s.list.FilterState() == list.Filtering }
func (s *ReposScreen) View(*core.Shared) string        { return s.list.View() }
func (s *ReposScreen) HelpView(*core.Shared) string    { return core.ShortHelp(s.list, core.HelpTabbed) }
func (s *ReposScreen) SetSize(_ *core.Shared, w, h int) { s.list.SetSize(w, h) }
func (s *ReposScreen) CrumbLabel(bool) string          { return "Repos" }

func (s *ReposScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	// The tab's own keys, gated behind the filter guard so they don't hijack filter typing.
	if k, ok := msg.(tea.KeyMsg); ok && !s.Filtering() {
		switch {
		// "V" opens the all-repos git page (fetch/pull/push across every scanned repo). The
		// per-repo page is enter on a row (the row's own Pick).
		case core.MatchKey(k.String(), keys.GitAll):
			if len(Of(sh).Repos) == 0 {
				return s, core.SetStatus("no repos to act on")
			}
			return s, core.Push(repoui.AllReposMenu(sh, allScope()))
		// "f" fetches every repo concurrently so the ahead/behind markers can see new upstream
		// commits. Network-bound, hence explicit.
		case core.MatchKey(k.String(), keys.Fetch):
			if s.fetching {
				return s, core.SetStatus("fetch already running")
			}
			if len(Of(sh).Repos) == 0 {
				return s, core.SetStatus("no repos to fetch")
			}
			s.fetching = true
			return s, core.Seq(
				core.SetStatus("fetching repos…"),
				core.Async(fetchAllCmd(sh)),
			)
		// "a" opens the small Actions menu (theme, refresh).
		case core.MatchKey(k.String(), keys.Actions):
			return s, core.Push(actionsMenu(sh))
		}
	}
	return s, components.RootUpdate(sh, &s.list, msg)
}

// Receive rebuilds the list from a fresh scan on repoview's own RescanMsg (the Refresh
// action / "r") or the shared git flows' repoui.RefreshMsg (raised after a pull/push/commit/
// single fetch) — both mean "the tree changed, re-read it". fetchDone additionally logs each
// repo's outcome and summarizes.
func (s *ReposScreen) Receive(sh *core.Shared, payload any) core.Action {
	switch p := payload.(type) {
	case repoui.RefreshMsg, RescanMsg:
		Of(sh).Scan()
		s.list.SetItems(repoListItems(sh))
	case fetchDone:
		s.fetching = false
		for _, r := range p.results {
			sh.Log(fetchLine(r))
		}
		Of(sh).Scan()
		s.list.SetItems(repoListItems(sh))
		if len(p.results) == 0 {
			return core.SetStatus("no repos fetched")
		}
		line, failed := fetchSummary(p.results)
		return core.SetStatusAndLog(line, failed) // force the log open only to show a failure
	}
	return core.Action{}
}

// allScope is the single "repos" scope handed to the all-repos menu: every scanned repo, read
// fresh. With one scope the menu shows no scope-cycle row (gated in repoui by len(scopes) > 1).
func allScope() []repoui.Scope {
	return []repoui.Scope{{
		Label: "repos",
		Repos: func(sh *core.Shared) []repo.Repo { return Of(sh).Repos },
	}}
}

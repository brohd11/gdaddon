// Package git supplies the scopes for the shared all-repos git menu (repoui.AllReposMenu),
// reached from Actions ▸ Git and "V" on the Project list. Scopes are gdaddon's: clones are
// repos you develop; submodules are parent-managed (pulling one dirties the parent), so
// acting on them is opt-in. The project root rides the include-root toggle, excluded from
// the submodules scope.
package git

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"
	"github.com/brohd11/gitstack/repoui"
)

// scope selects which checkouts the batch operations act on; clones by default.
type scope int

const (
	scopeClones scope = iota
	scopeSubmodules
	scopeAll
)

func (s scope) label() string {
	switch s {
	case scopeSubmodules:
		return "submodules"
	case scopeAll:
		return "all"
	default:
		return "clones"
	}
}

// matches reports whether an entry is a git checkout this scope selects. "all" still means
// git checkouts only — a package is never one.
func (s scope) matches(a addon.Addon) bool {
	switch s {
	case scopeClones:
		return a.IsClone()
	case scopeSubmodules:
		return a.IsSubmodule()
	default:
		return a.IsGitWorkdir()
	}
}

// AllRepos is the project-wide git menu: fetch, pull or push every checkout in the chosen
// scope, with the project root as an include-root toggle (not under submodules).
func AllRepos(sh *core.Shared) *components.PickerScreen {
	return repoui.AllReposMenu(sh, []repoui.Scope{
		newScope(scopeClones),
		newScope(scopeSubmodules),
		newScope(scopeAll),
	}, repoui.RootOptionFor(func(sh *core.Shared) *repo.Repo { return appctx.Of(sh).RootRepo }))
}

// newScope builds one repoui.Scope from the project cache; the submodules scope excludes
// the root.
func newScope(sc scope) repoui.Scope {
	return repoui.Scope{
		Label:       sc.label(),
		Repos:       func(sh *core.Shared) []repo.Repo { return reposFor(sh, sc) },
		ExcludeRoot: sc == scopeSubmodules,
	}
}

// reposFor returns the cached checkouts present in scope with current divergence (the
// root comes from the toggle).
func reposFor(sh *core.Shared, sc scope) []repo.Repo {
	c := appctx.Of(sh)
	statuses := c.ProjectStatuses()
	var out []repo.Repo
	for _, s := range statuses {
		if !s.Addon.IsGitWorkdir() || !s.Present() || !sc.matches(s.Addon) {
			continue
		}
		out = append(out, repo.Repo{
			Name: s.Addon.Label(),
			Dir:  s.FullPath,
			Sync: c.GitSync[s.Addon.Name],
		})
	}
	return out
}

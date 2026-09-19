// Package git wires gdaddon's manifest to the domain-neutral all-repos git menu
// (repoui.AllReposMenu), shared with the standalone repo-viewer tooling. It is a flow rather
// than a tab package because two tabs reach it: Actions ▸ Git and "V" on the Project list.
//
// All the menu/confirm/batch machinery lives in repoui; this package only supplies the
// scopes — which manifest checkouts each operation acts on. The scope concept (clones vs
// submodules vs all) is gdaddon's: a clone is a repo you develop, a submodule is
// parent-managed (pulling one dirties the parent's recorded pointer), so acting on
// submodules is an opt-in the cycling scope makes explicit.
//
// The project root's own repo is no manifest entry; it rides along behind the shared menu's
// include-root toggle. But it's a top-level clone, so the submodules scope marks itself
// ExcludeRoot — the toggle won't add (or even offer) the root there, only under clones and all.
package git

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"
	"github.com/brohd11/gitstack/repoui"
)

// scope selects which git checkouts the all-repos operations act on. Default (first in the
// cycle) is clones — the repos you develop. Submodules are parent-managed, so acting on them
// is an opt-in step, not the default.
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

// AllRepos is the project-wide git menu: fetch, pull, or push every checkout in the manifest,
// narrowed by a cycling scope. Opened from Actions ▸ Git and "V" on the Project list. The
// three scopes are handed to the shared repoui menu, which owns the cycling, confirm, and
// batch execution. The project root's own repo rides along as an include-root toggle (it isn't
// a manifest entry, so no scope covers it): appctx.Ctx.RootRepo supplies it, and the menu
// appends it to the targets while the toggle is on — except under the submodules scope, which
// is ExcludeRoot (the root is a clone, not a submodule).
func AllRepos(sh *core.Shared) *components.PickerScreen {
	return repoui.AllReposMenu(sh, []repoui.Scope{
		newScope(scopeClones),
		newScope(scopeSubmodules),
		newScope(scopeAll),
	}, repoui.RootOptionFor(func(sh *core.Shared) *repo.Repo { return appctx.Of(sh).RootRepo }))
}

// newScope builds one repoui.Scope from the current project cache.
// The submodules scope opts out of the include-root toggle (ExcludeRoot): the
// project root is a top-level clone, so it has no place in a submodules-only batch.
func newScope(sc scope) repoui.Scope {
	return repoui.Scope{
		Label:       sc.label(),
		Repos:       func(sh *core.Shared) []repo.Repo { return reposFor(sh, sc) },
		ExcludeRoot: sc == scopeSubmodules,
	}
}

// reposFor returns cached present checkouts in scope, annotated with current
// divergence. Root refresh handlers update this cache before deeper menus receive
// a broadcast. The project root is supplied separately by the include-root toggle.
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

package project

import (
	"gdaddon/internal/addon"

	"github.com/brohd11/gitstack/repo"
)

// The per-addon Git submenu (see what changed, fetch, pull, commit, push) is the
// domain-neutral repoui.RepoMenu, shared with the standalone repo-viewer tooling. All this
// tab supplies is the adapter from a manifest Status to the repo.Repo that menu operates on;
// items.go wires "v" on a present git checkout to it. (The project root's own repo reaches
// the same menu through the all-repos flow's include-root toggle and the header's Root line,
// not through a list row.)

// repoFromStatus adapts an inspected manifest entry into the repo.Repo the shared git submenu
// renders. Branch falls back to the recorded tag when the live checkout can't be read, which
// is what the commit confirm's "on <branch>" header wants.
func repoFromStatus(s addon.Status) repo.Repo {
	branch := s.LiveBranch
	if branch == "" {
		branch = s.Addon.Tag
	}
	return repo.Repo{Name: s.Addon.Name, Dir: s.FullPath, Branch: branch}
}

package project

import (
	"github.com/brohd11/gdaddon/internal/addon"

	"github.com/brohd11/gitstack/repo"
)

// The per-addon Git menu is repoui.RepoMenu; this adapts a manifest Status to a repo.Repo.

// repoFromStatus adapts an entry for the Git menu; Branch falls back to the recorded tag.
func repoFromStatus(s addon.Status) repo.Repo {
	branch := s.LiveBranch
	if branch == "" {
		branch = s.Addon.Tag
	}
	return repo.Repo{Name: s.Addon.Label(), Dir: s.FullPath, Branch: branch}
}

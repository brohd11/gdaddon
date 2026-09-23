package addon

import (
	"context"

	"github.com/brohd11/gitstack/repo"
)

// The git engine lives in gitstack/repo; its types and functions are re-exported here for
// gdaddon's callers. FetchAll stays here because it knows the manifest.

// Reporter receives progress lines; aliased from the engine so install and git flows share
// it.
type Reporter = repo.Reporter

// Git status/result types, aliased from the engine.
type (
	GitSync     = repo.GitSync
	GitChange   = repo.GitChange
	FetchResult = repo.FetchResult
)

// Git engine functions, re-exported so addon.* callers are unaffected by the move.
var (
	GitStream             = repo.GitStream
	GitStatus             = repo.GitStatus
	GitPull               = repo.GitPull
	GitPush               = repo.GitPush
	GitCommit             = repo.GitCommit
	GitFetch              = repo.GitFetch
	GitSyncStatus         = repo.GitSyncStatus
	GitChanges            = repo.GitChanges
	HasUncommittedChanges = repo.HasUncommittedChanges
	CurrentBranch         = repo.CurrentBranch
	FindGitRepos          = repo.FindGitRepos
	FetchLine             = repo.FetchLine
	FetchSummary          = repo.FetchSummary
)

// FetchRepos maps the git checkouts present on disk to repo.Repo values for the engine (for
// repoui.FetchAllCmd), skipping everything else.
func FetchRepos(statuses []Status) []repo.Repo {
	repos := make([]repo.Repo, 0, len(statuses))
	for _, s := range statuses {
		if !s.Addon.IsGitWorkdir() || !s.Present() {
			continue
		}
		repos = append(repos, repo.Repo{Name: s.Addon.Label(), Dir: s.FullPath})
	}
	return repos
}

// FetchAll fetches every present git checkout among statuses and reads each one's post-fetch
// divergence — FetchRepos to select the checkouts, then the engine's repo.FetchAll.
func FetchAll(ctx context.Context, statuses []Status) []FetchResult {
	return repo.FetchAll(ctx, FetchRepos(statuses))
}

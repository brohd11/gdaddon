package addon

import (
	"context"

	"github.com/brohd11/gitstack/repo"
)

// The domain-neutral git engine lives in github.com/brohd11/gitstack/repo, shared with the
// standalone repo-viewer tooling. gdaddon re-exports its types and functions here under the
// addon package so the many existing callers (cmd/repos.go, the TUI, appctx) keep referring
// to addon.GitFetch/GitSync/… unchanged. FetchAll stays a gdaddon function — the one piece
// that knows the manifest — adapting []Status onto the engine's []repo.Repo.

// Reporter is a sink for human-readable progress lines. The CLI prints them to stdout; the
// TUI funnels them into bubbletea messages. Aliased from the engine so the install/update
// flows and the git flows name the same type.
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
	FindGitRepos          = repo.FindGitRepos
)

// FetchAll fetches every present git checkout (clone or submodule) among statuses and reads
// each one's post-fetch divergence. It's the manifest-aware adapter over the engine's
// repo.FetchAll: it filters to the git checkouts that are actually on disk (nothing else can
// be fetched) and maps each to a repo.Repo. Non-git and not-installed entries are skipped.
func FetchAll(ctx context.Context, statuses []Status) []FetchResult {
	repos := make([]repo.Repo, 0, len(statuses))
	for _, s := range statuses {
		if !s.Addon.IsGitWorkdir() || !s.Present() {
			continue
		}
		repos = append(repos, repo.Repo{Name: s.Addon.Name, Dir: s.FullPath})
	}
	return repo.FetchAll(ctx, repos)
}

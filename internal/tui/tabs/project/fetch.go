package project

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"

	tea "charm.land/bubbletea/v2"
)

// fetchAllCmd fetches every present checkout in the manifest plus the project root's repo,
// off the UI thread through the app-owned FetchState. The manifest is inspected inside
// the gather closure; paths and the root repo are captured on the UI thread.
func fetchAllCmd(sh *core.Shared) (tea.Cmd, bool) {
	c := appctx.Of(sh)
	manifestPath, projectRoot := c.ManifestPath, c.ProjectRoot
	var roots []repo.Repo
	if c.RootRepo != nil {
		roots = []repo.Repo{*c.RootRepo}
	}
	return c.Fetch.Start(func() []repo.Repo {
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return nil
		}
		repos := addon.FetchRepos(statuses)
		return append(repos, roots...)
	}, c.GitHookApp())
}

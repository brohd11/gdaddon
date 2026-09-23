package project

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"
	"github.com/brohd11/gitstack/repoui"

	tea "charm.land/bubbletea/v2"
)

// fetchAllCmd fetches every present checkout in the manifest plus the project root's repo,
// off the UI thread through repoui.FetchAllCmd. It is the explicit network step behind the
// ahead/behind markers. The manifest is inspected inside the gather closure, so nothing
// here touches Shared.
func fetchAllCmd(sh *core.Shared) tea.Cmd {
	c := appctx.Of(sh)
	manifestPath, projectRoot := c.ManifestPath, c.ProjectRoot
	root := c.RootRepo
	return repoui.FetchAllCmd(func() []repo.Repo {
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return nil
		}
		repos := addon.FetchRepos(statuses)
		if root != nil {
			repos = append(repos, *root)
		}
		return repos
	})
}

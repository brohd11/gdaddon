package project

import (
	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"
	"github.com/brohd11/gitstack/repoui"

	tea "github.com/charmbracelet/bubbletea"
)

// fetchAllCmd runs `git fetch` in every present git checkout the manifest tracks — plus the
// project root's own repo, which is no manifest entry but is fetched unconditionally here —
// off the UI thread, via the shared repoui fan-out (repoui.FetchAllCmd bakes in the timeout,
// concurrency, and FetchDoneMsg broadcast). It's the announced network step behind the
// ahead/behind markers: those counts are read locally on every refresh, but only a fetch can
// make them notice new upstream commits — so this is bound to a key rather than riding the
// inspect pass.
//
// The manifest paths and the cached root repo are captured up front; the manifest inspect
// runs inside the gather closure (which repoui calls on the goroutine, off the UI thread), so
// nothing here touches Shared. The root repo just rides along in the repo set — repo.FetchAll
// builds the same per-repo result for it as for a manifest checkout.
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

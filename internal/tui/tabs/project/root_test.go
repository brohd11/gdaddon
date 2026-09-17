package project

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gitstack/repoui"

	tea "charm.land/bubbletea/v2"
)

func TestProjectRootAsyncCallbacks(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "addon_manifest.yml")
	if err := addon.AddEntry(manifest, "alpha", "https://github.com/example/alpha", "addons/alpha"); err != nil {
		t.Fatal(err)
	}
	c := &appctx.Ctx{ProjectRoot: dir, ManifestPath: manifest}
	sh := core.NewShared(c)
	root := NewProjectScreen(sh)
	root.SetSize(sh, 60, 20)
	if root.Init(sh) == nil {
		t.Fatal("initialization must schedule the update check")
	}
	for _, payload := range []any{appctx.ProjectDirty{}, appctx.PathRefresh{}} {
		if act := root.Receive(sh, payload); act.Cmd == nil {
			t.Fatalf("%T must schedule a fresh update check", payload)
		}
	}
	root.List().SetFilterText("alpha")
	root.SetCompact(true)
	checks := map[string]addon.UpdateInfo{"alpha": {State: addon.UpdateAvailable, LatestTag: "v2"}}
	root.Receive(sh, updateChecksReady{checks: checks})
	if c.UpdateChecks["alpha"].LatestTag != "v2" || len(root.List().VisibleItems()) != 1 || !root.Compact() {
		t.Fatal("async results must update the cache without dropping filter or density")
	}
	if act := root.Receive(sh, appctx.GitRefresh{}); act.Cmd != nil {
		t.Fatal("a local Git refresh must not schedule a network update check")
	}
}

func TestProjectRootFetchGuardSurvivesUpdates(t *testing.T) {
	sh := core.NewShared(&appctx.Ctx{ProjectRoot: t.TempDir()})
	sh.Chrome = &core.Chrome{Status: components.NewStatusLine()}
	r := core.NewRouter(sh, []core.TabEntry{{Title: "Project", New: func(sh *core.Shared) core.Screen {
		return NewProjectScreen(sh)
	}}})
	// Do not execute the returned commands: this checks scheduling and completion
	// routing without doing any network work or waiting for status timers.
	fetch := tea.KeyPressMsg{Code: 'f', Text: "f"}
	m, cmd := r.Update(fetch)
	r = m.(core.Router)
	if cmd == nil || !strings.Contains(sh.Chrome.Status.View(), "fetching git") {
		t.Fatal("fetch should be scheduled and announced")
	}
	m, _ = r.Update(fetch)
	r = m.(core.Router)
	if !strings.Contains(sh.Chrome.Status.View(), "already running") {
		t.Fatal("repeated input must not discard the in-flight fetch guard")
	}
	m, _ = r.Update(core.PropagateAll(repoui.FetchDoneMsg{}))
	r = m.(core.Router)
	m, cmd = r.Update(fetch)
	if cmd == nil || !strings.Contains(sh.Chrome.Status.View(), "fetching git") {
		t.Fatal("fetch completion must allow another fetch")
	}
	if _, ok := m.(core.Router).Top().(*components.RootListScreen); !ok {
		t.Fatal("fetch completion must rebuild the shared root component")
	}
}

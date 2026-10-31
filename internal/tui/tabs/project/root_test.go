package project

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gitstack/repo"
	"github.com/brohd11/gitstack/repoui"

	tea "charm.land/bubbletea/v2"
)

func queuedFetchCompletion(t *testing.T, state *repoui.FetchState) repoui.FetchDoneMsg {
	t.Helper()
	cmd, ok := state.Start(func() []repo.Repo { return nil })
	if !ok {
		t.Fatal("fetch already active")
	}
	var done repoui.FetchDoneMsg
	count := 0
	core.DrainBroadcasts(core.Async(cmd), func(payload any) core.Action {
		done = payload.(repoui.FetchDoneMsg)
		count++
		return core.Action{}
	})
	if count != 1 {
		t.Fatalf("got %d completions", count)
	}
	return done
}

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
		before := c.UpdateGen.Load()
		root.Receive(sh, payload)
		if c.UpdateGen.Load() == before {
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
	before := c.UpdateGen.Load()
	root.Receive(sh, appctx.GitRefresh{})
	if c.UpdateGen.Load() != before {
		t.Fatal("a local Git refresh must not schedule a network update check")
	}
}

func TestProjectRootFetchGuardSurvivesUpdates(t *testing.T) {
	c := &appctx.Ctx{ProjectRoot: t.TempDir()}
	sh := core.NewShared(c)
	sh.Chrome = &core.Chrome{Status: components.NewStatusLine()}
	r := core.NewRouter(sh, []core.TabEntry{{Title: "Project", New: func(sh *core.Shared) core.Screen {
		return NewProjectScreen(sh)
	}}})
	// Queue an owned empty result without network work or status timers.
	done := queuedFetchCompletion(t, &c.Fetch)
	fetch := tea.KeyPressMsg{Code: 'f', Text: "f"}
	m, _ := r.Update(fetch)
	r = m.(core.Router)
	if !strings.Contains(sh.Chrome.Status.View(), "already running") {
		t.Fatal("repeated input must not discard the in-flight fetch guard")
	}
	root := r.Top()
	m, _ = r.Update(core.PropagateAll(done))
	r = m.(core.Router)
	if r.Top() == root {
		t.Fatal("fetch completion must rebuild the root")
	}
	m, cmd := r.Update(fetch)
	if cmd == nil || !strings.Contains(sh.Chrome.Status.View(), "fetching git") {
		t.Fatal("fetch completion must allow another fetch")
	}
	if _, ok := m.(core.Router).Top().(*components.RootListScreen); !ok {
		t.Fatal("fetch completion must rebuild the shared root component")
	}
}

func TestFetchGuardSurvivesThemeRebuild(t *testing.T) {
	sh := core.NewShared(&appctx.Ctx{ProjectRoot: t.TempDir()})
	sh.Chrome = &core.Chrome{Status: components.NewStatusLine()}
	r := core.NewRouter(sh, []core.TabEntry{{Title: "Project", New: func(sh *core.Shared) core.Screen {
		return NewProjectScreen(sh)
	}}})
	step := func(msg tea.Msg) {
		m, _ := r.Update(msg)
		r = m.(core.Router)
	}
	root := r.Top()
	step(tea.KeyPressMsg{Code: 'f', Text: "f"})
	step(core.PropagateAll(core.MsgThemeChanged{}))
	if r.Top() == root {
		t.Fatal("theme change must rebuild the root")
	}
	step(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if !strings.Contains(sh.Chrome.Status.View(), "already running") {
		t.Fatal("theme rebuild lost the in-flight fetch guard")
	}
}

type fetchLog struct {
	*components.LogPane
	lines []string
}

func (l *fetchLog) Log(line string, force bool) { l.lines = append(l.lines, line) }

func TestFetchCompletionOwnershipAcrossNavigation(t *testing.T) {
	for _, inactive := range []bool{false, true} {
		name := "covered"
		if inactive {
			name = "inactive"
		}
		t.Run(name, func(t *testing.T) {
			c := &appctx.Ctx{ProjectRoot: t.TempDir()}
			sh := core.NewShared(c)
			log := &fetchLog{LogPane: components.NewLogPane()}
			sh.Chrome = &core.Chrome{Status: components.NewStatusLine(), Output: log}
			var root *components.RootListScreen
			r := core.NewRouter(sh, []core.TabEntry{
				{Title: "Project", New: func(sh *core.Shared) core.Screen { root = NewProjectScreen(sh); return root }},
				{Title: "Other", New: func(*core.Shared) core.Screen { return components.NewPicker(nil, components.PickerOpts{}) }},
			})
			step := func(msg tea.Msg) { m, _ := r.Update(msg); r = m.(core.Router) }
			first := queuedFetchCompletion(t, &c.Fetch)
			first.Results = []repo.FetchResult{{Name: "owned", Err: errors.New("fetch failed"), Output: []string{"worker output"}}}
			if inactive {
				step(core.ShowTab("Other"))
			} else {
				step(core.Push(components.NewPicker(nil, components.PickerOpts{})))
			}
			oldRoot := root
			step(core.PropagateAll(core.MsgThemeChanged{}))
			if root == oldRoot || !c.Fetch.Running() {
				t.Fatal("rebuild must replace the root and retain fetch ownership")
			}
			var foreign repoui.FetchState
			other := queuedFetchCompletion(t, &foreign)
			reject := func(done repoui.FetchDoneMsg) {
				t.Helper()
				marker, _ := c.Refresh.Start(1)
				sh.WriteStatus("keep status")
				top, currentRoot, lines := r.Top(), root, len(log.lines)
				step(core.PropagateAll(done))
				if !c.Fetch.Running() || c.Refresh.Stale(marker) || r.Top() != top || root != currentRoot ||
					len(log.lines) != lines || !strings.Contains(sh.Chrome.Status.View(), "keep status") {
					t.Fatal("rejected completion changed guard, refresh, navigation, log or status")
				}
			}
			reject(repoui.FetchDoneMsg{Results: first.Results})
			reject(other)
			before := root
			step(core.PropagateAll(first))
			if c.Fetch.Running() || len(log.lines) != 3 || root == before {
				t.Fatalf("owned failure must release, log and rebuild: running=%v, log=%q", c.Fetch.Running(), log.lines)
			}
			second := queuedFetchCompletion(t, &c.Fetch)
			reject(first)
			reject(other)
			step(core.PropagateAll(second))
			if c.Fetch.Running() {
				t.Fatal("owned empty completion must release the next run")
			}
			step(core.ShowTab("Project"))
			step(tea.KeyPressMsg{Code: 'f', Text: "f"})
			if !c.Fetch.Running() || !strings.Contains(sh.Chrome.Status.View(), "fetching git") {
				t.Fatal("completion must allow a new shortcut fetch")
			}
		})
	}
}

package project

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func projectBatchFixture(t *testing.T) (*appctx.Ctx, *core.Shared, *projectState, []any) {
	t.Helper()
	dir := t.TempDir()
	manifest := ""
	for _, name := range []string{"alpha", "beta", "gamma"} {
		manifest += fmt.Sprintf("%s:\n  url: https://example.invalid/%s.zip\n  path: addons/%s\n  version: 1.0.0\n", name, name, name)
		if name != "beta" {
			path := filepath.Join(dir, "addons", name)
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "plugin.cfg"), []byte("[plugin]\nversion=\"1.0.0\"\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "addon_manifest.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	c := appctx.NewDeferred(dir, "test")
	sh := core.NewShared(c)
	s := &projectState{sort: appctx.SortStatusInstalled, screen: components.NewRootList(nil, components.RootListOpts{})}
	s.screen.SetSize(sh, 100, 30)
	var results []any
	core.DrainBroadcasts(s.reload(sh), func(payload any) core.Action {
		if _, tick := payload.(components.RefreshTickMsg); !tick {
			results = append(results, payload)
		}
		return core.Action{}
	})
	if len(results) != 4 {
		t.Fatalf("got %d inspection results, want 4", len(results))
	}
	return c, sh, s, results
}

func queuedPublication(t *testing.T, act core.Action) any {
	t.Helper()
	var tick any
	core.DrainBroadcasts(act, func(payload any) core.Action {
		if tick != nil {
			t.Fatal("burst scheduled multiple publications")
		}
		tick = payload
		return core.Action{}
	})
	if tick == nil {
		t.Fatal("no publication scheduled")
	}
	return tick
}

func TestProjectBatchVisibilityAndFinalPublication(t *testing.T) {
	c, sh, s, results := projectBatchFixture(t)
	s.screen.List().SetFilterText("a")
	s.screen.List().SetFilterState(list.Filtering)
	var tick any
	i := 0
	for ; i < len(results); i++ {
		act := s.receive(sh, results[i])
		if len(c.DirtyProjectRows) > 0 {
			tick = queuedPublication(t, act)
			i++
			break
		}
	}
	if tick == nil || !c.Refresh.Running() {
		t.Fatal("first addon result must schedule an intermediate batch")
	}
	unchanged := ""
	for _, st := range c.ProjectStatuses() {
		if c.Pending[st.Addon.Name] {
			unchanged = st.Addon.Name
			break
		}
	}
	row := s.rows[unchanged].(components.Item)
	row.Desc = "cached sentinel"
	s.rows[unchanged] = row
	filters := 0
	s.screen.List().Filter = func(term string, targets []string) []list.Rank {
		filters++
		return list.DefaultFilter(term, targets)
	}
	s.receive(sh, tick)
	if filters != 1 || s.rows[unchanged].(components.Item).Desc != "cached sentinel" {
		t.Fatal("batch must filter once and retain unchanged rows")
	}
	for _, st := range c.ProjectStatuses() {
		if !c.Pending[st.Addon.Name] && s.rows[st.Addon.Name].(components.Item).Pending {
			t.Fatal("landed row remains pending after publication")
		}
	}
	for ; i < len(results); i++ {
		s.receive(sh, results[i])
	}
	items := s.screen.List().Items()
	if c.Refresh.Running() || c.RootPending || len(c.Pending) != 0 || len(items) != 2 || c.RankSnap != nil {
		t.Fatal("final publication must finish progress, clear pending and hide missing addons")
	}
	if items[0].(components.Item).Key != "alpha" || items[1].(components.Item).Key != "gamma" {
		t.Fatalf("unexpected final order: %+v", items)
	}
	if s.screen.List().FilterState() != list.Filtering || s.screen.List().FilterValue() != "a" {
		t.Fatal("publication lost live filtering")
	}
	for _, item := range items {
		row := item.(components.Item)
		if row.Pending || row.Pick == nil {
			t.Fatal("installed row did not publish its current actions and state")
		}
	}
	s.receive(sh, tick)
	if &s.screen.List().Items()[0] != &items[0] {
		t.Fatal("stale timer republished a completed list")
	}
}

func TestProjectPublicationAcrossRootRebuild(t *testing.T) {
	for _, inactive := range []bool{false, true} {
		t.Run(fmt.Sprint(inactive), func(t *testing.T) {
			c, sh, _, results := projectBatchFixture(t)
			var root *components.RootListScreen
			r := core.NewRouter(sh, []core.TabEntry{
				{Title: "Project", New: func(sh *core.Shared) core.Screen { root = NewProjectScreen(sh); return root }},
				{Title: "Other", New: func(*core.Shared) core.Screen { return components.NewPicker(nil, components.PickerOpts{}) }},
			})
			step := func(msg tea.Msg) tea.Cmd { m, cmd := r.Update(msg); r = m.(core.Router); return cmd }
			var tick any
			i := 0
			for ; i < len(results); i++ {
				cmd := step(core.PropagateAll(results[i]))
				if len(c.DirtyProjectRows) > 0 {
					tick = queuedPublication(t, core.Async(cmd))
					i++
					break
				}
			}
			if inactive {
				step(core.ShowTab("Other"))
			} else {
				step(core.Push(components.NewPicker(nil, components.PickerOpts{})))
			}
			before := root
			step(core.PropagateAll(core.MsgThemeChanged{}))
			if root == before {
				t.Fatal("theme did not rebuild the root")
			}
			items := root.List().Items()
			step(core.PropagateAll(tick))
			if &root.List().Items()[0] != &items[0] {
				t.Fatal("old screen's tick published to the new root")
			}
			for ; i < len(results); i++ {
				step(core.PropagateAll(results[i]))
			}
			if c.Refresh.Running() {
				t.Fatal("covered or inactive root lost refresh completion")
			}
			for _, row := range root.List().Items() {
				if row.(components.Item).Pending {
					t.Fatal("root rebuild left stale pending rows")
				}
			}
		})
	}
}

func TestUpdateChecksPublishQueuedInspectionsImmediately(t *testing.T) {
	c, sh, s, results := projectBatchFixture(t)
	var tick any
	for _, result := range results {
		act := s.receive(sh, result)
		if len(c.DirtyProjectRows) > 0 {
			tick = queuedPublication(t, act)
			break
		}
	}
	s.receive(sh, updateChecksReady{checks: map[string]addon.UpdateInfo{"alpha": {State: addon.UpdateAvailable}}})
	items := s.screen.List().Items()
	if len(c.DirtyProjectRows) != 0 {
		t.Fatal("update-check publication did not incorporate queued inspections")
	}
	s.receive(sh, tick)
	if &s.screen.List().Items()[0] != &items[0] {
		t.Fatal("retired timer republished after an update-check result")
	}
}

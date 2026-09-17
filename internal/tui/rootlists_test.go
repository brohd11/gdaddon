package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/tabs/actions"
	"github.com/brohd11/gdaddon/internal/tui/tabs/archive"
	"github.com/brohd11/gdaddon/internal/tui/tabs/global"
	"github.com/brohd11/gdaddon/internal/tui/tabs/project"
	"github.com/brohd11/gdaddon/internal/tui/tabs/search"
	"github.com/brohd11/gdaddon/internal/tui/tabs/sets"

	"charm.land/bubbles/v2/list"
)

func TestAllRootListsShareDensityAndKeepCallbacks(t *testing.T) {
	projectDir := t.TempDir()
	manifest := filepath.Join(projectDir, "addon_manifest.yml")
	for _, name := range []string{"alpha", "zeta"} {
		if err := addon.AddEntry(manifest, name, "https://github.com/example/"+name, "addons/"+name); err != nil {
			t.Fatal(err)
		}
	}
	c := &appctx.Ctx{ProjectRoot: projectDir, ManifestPath: manifest}
	sh := core.NewShared(c)
	cases := []struct {
		name  string
		new   func(*core.Shared) *components.RootListScreen
		dirty any
		sort  bool
	}{
		{"Project", project.NewProjectScreen, appctx.ProjectDirty{}, true},
		{"Global", global.NewGlobalScreen, appctx.GlobalDirty{}, true},
		{"Sets", sets.NewSetsScreen, appctx.SetsDirty{}, true},
		{"Archive", archive.NewArchiveScreen, appctx.ArchiveDirty{}, true},
		{"Actions", actions.NewActionsScreen, appctx.PathRefresh{}, false},
		{"Search", search.NewSearchScreen, nil, false},
	}
	roots := make([]*components.RootListScreen, len(cases))
	for i, tc := range cases {
		roots[i] = tc.new(sh)
		roots[i].SetSize(sh, 80, 24)
		if roots[i].Compact() {
			t.Fatalf("%s should initially be expanded", tc.name)
		}
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := roots[i]
			top, _ := root.Update(sh, keyMsg(densityKey()))
			if top != root || !c.Compact || !root.Compact() {
				t.Fatal("toggle must retain the root and update the shared preference")
			}
			for j, other := range roots {
				other.SetSize(sh, 80, 24)
				if !other.Compact() {
					t.Fatalf("toggle did not reach %s", cases[j].name)
				}
			}
			rebuilt := tc.new(sh)
			rebuilt.SetSize(sh, 80, 24) // first lifecycle call binds the app preference
			if !rebuilt.Compact() {
				t.Fatal("root reconstruction must preserve density")
			}
			// Each tab starts with at least a placeholder row. Filter to that row,
			// then sort/refresh to exercise its real domain callbacks.
			selected := components.SelectedTitle(root.List())
			if selected == "" {
				t.Fatal("expected a row or placeholder")
			}
			root.List().SetFilterText(selected)
			if tc.sort {
				root.Update(sh, keyMsg(appctx.AppKeys.Sort.Keys()[0]))
				if !strings.Contains(root.List().Title, "Z→A") {
					t.Fatalf("sort callback did not retitle the list: %q", root.List().Title)
				}
			}
			if tc.dirty != nil {
				// A temporary sentinel proves the callback actually rebuilt the rows.
				rows := append([]list.Item{}, root.List().Items()...)
				root.SetItems(append(rows, components.Item{Name: "rootlist-probe"}))
				root.Receive(sh, tc.dirty)
				for _, row := range root.List().Items() {
					if row.FilterValue() == "rootlist-probe" {
						t.Fatal("dirty broadcast did not reload the rows")
					}
				}
			}
			if root.List().FilterValue() != selected || len(root.List().VisibleItems()) == 0 {
				t.Fatal("sorting and refresh must retain a populated filter")
			}
			if tc.name == "Sets" && root.List().Items()[0].(components.Item).Name != "+ New set" {
				t.Fatal("sort and refresh must retain the pinned New set row")
			}
			root.Update(sh, keyMsg(densityKey()))
			if c.Compact {
				t.Fatal("toggle must also return the session preference to expanded")
			}
		})
	}
}

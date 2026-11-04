package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/tabs/actions"
	"github.com/brohd11/gdaddon/internal/tui/tabs/project"
)

func TestNestedProjectRouterSmoke(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, layout := range []string{"repository manifest", "tools manifest"} {
		t.Run(layout, func(t *testing.T) {
			base := t.TempDir()
			projectRoot := filepath.Join(base, "godot")
			write := func(path, data string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			manifest, rootField := filepath.Join(base, "addon_manifest.yml"), "godot"
			if layout == "tools manifest" {
				manifest, rootField = filepath.Join(projectRoot, "tools", "addon_manifest.yml"), ".."
			}
			write(manifest, "project_root: "+rootField+"\ndemo:\n  url: https://github.com/u/demo\n  path: addons/demo\n")
			write(filepath.Join(projectRoot, "project.godot"), "[application]\nconfig/name=\"Nested Demo\"\n")
			write(filepath.Join(projectRoot, "addons", "demo", "plugin.cfg"), "[plugin]\nversion=\"1.0.0\"\n")
			c := appctx.NewDeferred(base, "test")
			sh := core.NewShared(c)
			sh.Chrome = &core.Chrome{Header: core.NewHeaderPane(appctx.Header), Output: components.NewLogPane()}
			tm := sized(core.NewRouter(sh, []core.TabEntry{
				{Title: "Project", New: func(sh *core.Shared) core.Screen { return project.NewProjectScreen(sh) }},
				{Title: "Actions", New: func(sh *core.Shared) core.Screen { return actions.NewActionsScreen(sh) }},
			}))
			root := tm.(core.Router).Top().(*components.RootListScreen)
			core.DrainBroadcasts(c.StartProjectRefresh(), func(payload any) core.Action { return root.Receive(sh, payload) })
			if out := view(tm); !strings.Contains(out, "Nested Demo") || !strings.Contains(out, "demo") || strings.Contains(out, "project_root") {
				t.Fatalf("incorrect project header or addon rows:\n%s", out)
			}
			tm = pump(tm, keyMsg("]"))
			if out := view(tm); !strings.Contains(out, "Install/Update All") || strings.Contains(out, "Create Manifest") {
				t.Fatalf("incorrect manifest actions:\n%s", out)
			}
		})
	}
}

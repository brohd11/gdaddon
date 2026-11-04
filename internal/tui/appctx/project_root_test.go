package appctx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
)

func writeProjectFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRootDiscoveryAndProjectInfo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, tc := range []struct{ name, manifestDir, field, projectDir string }{
		{"repository manifest", ".", "project_root: godot\n", "godot"},
		{"tools manifest", "godot/tools", "project_root: ..\n", "godot"},
		{"nested default", "godot", "", "godot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			project := filepath.Join(base, tc.projectDir)
			manifest := filepath.Join(base, tc.manifestDir, "addon_manifest.yml")
			writeProjectFile(t, manifest, tc.field+"demo:\n  url: https://github.com/u/demo\n  path: addons/demo\n  version: '1.0'\n")
			writeProjectFile(t, filepath.Join(base, "rust", "Cargo.toml"), "[package]\n")
			writeProjectFile(t, filepath.Join(project, "project.godot"), "[application]\nconfig/name=\"Nested Project\"\n")
			writeProjectFile(t, filepath.Join(project, "addons", "demo", "plugin.cfg"), "[plugin]\nversion=\"1.0\"\n")
			c := New(base, "test")
			for range 2 {
				if c.SearchRoot != base || c.ProjectRoot != project || c.ManifestPath != manifest || c.ProjectName != "Nested Project" || !c.HasProject {
					t.Fatalf("wrong paths or project info: root=%s search=%s manifest=%s name=%s", c.ProjectRoot, c.SearchRoot, c.ManifestPath, c.ProjectName)
				}
				statuses := c.ProjectStatuses()
				if len(statuses) != 1 || statuses[0].State != addon.StateInstalled || statuses[0].FullPath != filepath.Join(project, "addons", "demo") {
					t.Fatalf("wrong install location: %+v", statuses)
				}
				c.Scan()
				c.RefreshProject()
			}
		})
	}
}

func TestRootChangeInvalidatesCachesAndPendingResults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	base := t.TempDir()
	manifest := filepath.Join(base, "addon_manifest.yml")
	const entry = "demo:\n  url: https://github.com/u/demo\n  path: addons/demo\n"
	writeProjectFile(t, manifest, "project_root: old\n"+entry)
	writeProjectFile(t, filepath.Join(base, "old", "addons", "demo", "plugin.cfg"), "[plugin]\nversion=\"old\"\n")
	writeProjectFile(t, filepath.Join(base, "new", "addons", "demo", "plugin.cfg"), "[plugin]\nversion=\"new\"\n")
	c := New(base, "test")
	previous := c.ProjectStatuses()[0]
	oldGen, _ := c.Refresh.Start(2)
	oldUpdateGen := c.UpdateGen.Load()
	c.UpdateChecks = map[string]addon.UpdateInfo{"demo": {State: addon.UpdateAvailable}}
	c.GitDirty = map[string]bool{"demo": true}
	writeProjectFile(t, manifest, "project_root: new\n"+entry)
	act := c.StartProjectRefresh()
	if c.ProjectRoot != filepath.Join(base, "new") || !c.Unseen("demo") || len(c.GitDirty) != 0 || len(c.UpdateChecks) != 0 || c.UpdateGen.Load() == oldUpdateGen {
		t.Fatal("changing root retained inspections or update results from the old directory")
	}
	if _, changed, _ := c.ApplyRefreshResult(addonInspectedMsg{gen: oldGen, status: previous}); changed {
		t.Fatal("accepted a pending result from the previous project root")
	}
	core.DrainBroadcasts(act, func(payload any) core.Action {
		_, _, action := c.ApplyRefreshResult(payload)
		return action
	})
	statuses := c.ProjectStatuses()
	if c.Refresh.Running() || len(statuses) != 1 || statuses[0].LocalVersion != "new" {
		t.Fatalf("new root failed to refresh: %+v", statuses)
	}
	c.Scan()
	if c.ManifestPath != manifest {
		t.Fatal("rescan lost the manifest above the project root")
	}
}

func TestNestedProjectKeepsEnclosingGitRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	base := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", base).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s, %v", out, err)
	}
	writeProjectFile(t, filepath.Join(base, "addon_manifest.yml"), "project_root: godot\n")
	writeProjectFile(t, filepath.Join(base, "godot", "project.godot"), "[application]\n")
	c := New(base, "test")
	want, _ := filepath.EvalSymlinks(base)
	if c.RootRepo == nil || c.RootRepo.Dir != want {
		t.Fatalf("expected enclosing repository %s, got %+v", want, c.RootRepo)
	}
	if !c.RefreshRepo(GitRepoRefresh{Dir: c.RootRepo.Dir}) {
		t.Fatal("enclosing repository operation must reload the whole project")
	}
}

func TestAsyncPathDiscoveryAppliesOnReceive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	base := t.TempDir()
	c := NewDeferred(base, "test")
	sh := core.NewShared(c)
	manifest := filepath.Join(base, "tools", "addon_manifest.yml")
	writeProjectFile(t, manifest, "project_root: ..\n")
	msg := RefreshPaths(sh, true)()
	if c.ManifestPath != "" {
		t.Fatal("background discovery mutated the context")
	}
	core.DrainBroadcasts(core.Async(func() tea.Msg { return msg }), func(payload any) core.Action {
		return c.Receive(sh, payload)
	})
	if c.ManifestPath != manifest || c.ProjectRoot != base {
		t.Fatalf("paths were not applied on receive: %s, %s", c.ManifestPath, c.ProjectRoot)
	}
}
